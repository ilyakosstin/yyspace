package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const maxMarkdownBytes = 1 << 20

type vault struct {
	folders, documents, revisions *mongo.Collection
}

type vaultFolder struct {
	ID   bson.ObjectID `bson:"_id" json:"id"`
	Name string        `bson:"name" json:"name"`
}

type vaultDocument struct {
	ID        bson.ObjectID  `bson:"_id" json:"id"`
	Revision  bson.ObjectID  `bson:"revision" json:"revision"`
	Title     string         `bson:"title" json:"title"`
	FolderID  *bson.ObjectID `bson:"folder_id" json:"folder_id"`
	UpdatedAt time.Time      `bson:"updated_at" json:"updated_at"`
}

type vaultRevision struct {
	ID       bson.ObjectID  `bson:"_id"`
	Document bson.ObjectID  `bson:"document_id"`
	Title    string         `bson:"title"`
	FolderID *bson.ObjectID `bson:"folder_id"`
	Markdown string         `bson:"markdown"`
}

type vaultContent struct {
	vaultDocument
	Markdown string `json:"markdown"`
	Merged   bool   `json:"merged"`
}

type vaultInput struct {
	Title    string         `json:"title"`
	FolderID *bson.ObjectID `json:"folder_id"`
	Markdown *string        `json:"markdown"`
	Revision bson.ObjectID  `json:"revision"`
}

func openVault(ctx context.Context, db *mongo.Database) (*vault, error) {
	v := &vault{db.Collection("vault_folders"), db.Collection("vault_documents"), db.Collection("vault_revisions")}
	_, err := v.folders.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}
	_, err = v.documents.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "folder_id", Value: 1}, {Key: "title", Value: 1}, {Key: "_id", Value: 1}},
	})
	return v, err
}

func vaultName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 160 || strings.ContainsAny(name, "\r\n") {
		return "", invalid("Enter a name between 1 and 160 characters on one line")
	}
	return name, nil
}

func vaultID(value string) (bson.ObjectID, error) {
	id, err := bson.ObjectIDFromHex(value)
	if err != nil {
		return id, invalid("Invalid vault ID")
	}
	return id, nil
}

func (v *vault) list(_ http.ResponseWriter, r *http.Request) (any, error) {
	var folder *bson.ObjectID
	if value := r.URL.Query().Get("folder"); value != "" {
		id, err := vaultID(value)
		if err != nil {
			return nil, err
		}
		folder = &id
	}
	ctx := r.Context()
	folders, err := readMany[vaultFolder](ctx, v.folders, bson.M{},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	documents, err := readMany[vaultDocument](ctx, v.documents, bson.M{"folder_id": folder},
		options.Find().SetSort(bson.D{{Key: "title", Value: 1}, {Key: "_id", Value: 1}}))
	return struct {
		Folders   []vaultFolder   `json:"folders"`
		Documents []vaultDocument `json:"documents"`
	}{folders, documents}, err
}

func (v *vault) createFolder(w http.ResponseWriter, r *http.Request) (any, error) {
	body, err := decode[struct{ Name string }](w, r)
	if err != nil {
		return nil, err
	}
	name, err := vaultName(body.Name)
	if err != nil {
		return nil, err
	}
	folder := vaultFolder{bson.NewObjectID(), name}
	_, err = v.folders.InsertOne(r.Context(), folder)
	if mongo.IsDuplicateKeyError(err) {
		return nil, conflict("A folder with that name already exists")
	}
	return folder, err
}

func (v *vault) document(ctx context.Context, id bson.ObjectID) (vaultDocument, error) {
	var document vaultDocument
	err := v.documents.FindOne(ctx, bson.M{"_id": id}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = notFound("Document")
	}
	return document, err
}

func (v *vault) revision(ctx context.Context, id, document bson.ObjectID) (vaultRevision, error) {
	var revision vaultRevision
	err := v.revisions.FindOne(ctx, bson.M{"_id": id, "document_id": document}).Decode(&revision)
	return revision, err
}

func (v *vault) read(r *http.Request, id bson.ObjectID) (vaultContent, error) {
	document, err := v.document(r.Context(), id)
	if err != nil {
		return vaultContent{}, err
	}
	rev, err := v.revision(r.Context(), document.Revision, id)
	return vaultContent{document, rev.Markdown, false}, err
}

func (v *vault) input(w http.ResponseWriter, r *http.Request) (*vaultInput, error) {
	body, err := decode[vaultInput](w, r)
	if err != nil {
		return nil, err
	}
	body.Title, err = vaultName(body.Title)
	if err != nil {
		return nil, err
	}
	if body.Markdown == nil || len(*body.Markdown) > maxMarkdownBytes {
		return nil, invalid("Markdown is required and can be up to 1 MiB")
	}
	if body.FolderID != nil {
		err = v.folders.FindOne(r.Context(), bson.M{"_id": *body.FolderID}).Err()
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, notFound("Folder")
		}
	}
	return body, err
}

func (v *vault) createDocument(w http.ResponseWriter, r *http.Request) (any, error) {
	body, err := v.input(w, r)
	if err != nil {
		return nil, err
	}
	document := vaultDocument{
		ID: bson.NewObjectID(), Revision: bson.NewObjectID(),
		Title: body.Title, FolderID: body.FolderID, UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	rev := vaultRevision{document.Revision, document.ID, body.Title, body.FolderID, *body.Markdown}
	ctx := r.Context()
	if _, err := v.revisions.InsertOne(ctx, rev); err != nil {
		return nil, err
	}
	_, err = v.documents.InsertOne(ctx, document)
	return vaultContent{document, rev.Markdown, false}, err
}

func folderValue(id *bson.ObjectID) bson.ObjectID {
	if id != nil {
		return *id
	}
	return bson.NilObjectID
}

func (v *vault) save(w http.ResponseWriter, r *http.Request, id bson.ObjectID) (vaultContent, error) {
	body, err := v.input(w, r)
	if err != nil {
		return vaultContent{}, err
	}
	ctx := r.Context()
	document, err := v.document(ctx, id)
	if err != nil {
		return vaultContent{}, err
	}
	base, err := v.revision(ctx, body.Revision, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return vaultContent{}, invalid("Load a valid document revision before saving")
	}
	if err != nil {
		return vaultContent{}, err
	}
	for {
		current, err := v.revision(ctx, document.Revision, id)
		if err != nil {
			return vaultContent{}, err
		}
		merged := base.ID != current.ID
		next := vaultRevision{bson.NewObjectID(), id, body.Title, body.FolderID,
			mergeMarkdown(base.Markdown, current.Markdown, *body.Markdown)}
		if body.Title == base.Title {
			next.Title = current.Title
		}
		if folderValue(body.FolderID) == folderValue(base.FolderID) {
			next.FolderID = current.FolderID
		}
		if len(next.Markdown) > maxMarkdownBytes {
			return vaultContent{}, invalid("The combined document exceeds 1 MiB. Your editor still contains your draft.")
		}
		if next.Title == current.Title && folderValue(next.FolderID) == folderValue(current.FolderID) &&
			next.Markdown == current.Markdown {
			return vaultContent{document, current.Markdown, merged}, nil
		}
		// Insert the immutable revision first. Readers can only observe it after the
		// compare-and-swap publishes its ID. Recompute the merge if another writer won.
		if _, err := v.revisions.InsertOne(ctx, next); err != nil {
			return vaultContent{}, err
		}
		updated := vaultDocument{id, next.ID, next.Title, next.FolderID, time.Now().UTC().Truncate(time.Millisecond)}
		result, err := v.documents.ReplaceOne(ctx, bson.M{"_id": id, "revision": current.ID}, updated)
		if err != nil {
			// An interrupted write may have published the revision, so keep it.
			return vaultContent{}, err
		}
		if result.MatchedCount == 1 {
			return vaultContent{updated, next.Markdown, merged}, nil
		}
		if _, err := v.revisions.DeleteOne(ctx, bson.M{"_id": next.ID}); err != nil {
			log.Printf("delete unpublished vault revision: %v", err)
		}
		document, err = v.document(ctx, id)
		if err != nil {
			return vaultContent{}, err
		}
	}
}

func vaultHandler(code int, handle func(http.ResponseWriter, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := handle(w, r)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, code, value)
	}
}

func (v *vault) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/vault", vaultHandler(http.StatusOK, v.list))
	mux.HandleFunc("POST /api/vault/folders", vaultHandler(http.StatusCreated, v.createFolder))
	mux.HandleFunc("POST /api/vault/documents", vaultHandler(http.StatusCreated, v.createDocument))
	mux.HandleFunc("GET /api/vault/documents/{id}", vaultHandler(http.StatusOK,
		func(_ http.ResponseWriter, r *http.Request) (any, error) {
			id, err := vaultID(r.PathValue("id"))
			if err != nil {
				return nil, err
			}
			return v.read(r, id)
		}))
	mux.HandleFunc("PUT /api/vault/documents/{id}", vaultHandler(http.StatusOK,
		func(w http.ResponseWriter, r *http.Request) (any, error) {
			id, err := vaultID(r.PathValue("id"))
			if err != nil {
				return nil, err
			}
			return v.save(w, r, id)
		}))
}
