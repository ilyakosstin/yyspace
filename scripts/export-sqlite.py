#!/usr/bin/env python3
"""Export the old Focus Helper SQLite database for the Go import command."""

import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import sqlite3
import sys


def records(db, table, dates):
    for row in db.execute(f"SELECT * FROM {table} ORDER BY id"):
        item = dict(row)
        for key in dates:
            if item.get(key):
                timestamp = datetime.fromisoformat(item[key])
                if timestamp.tzinfo is None:
                    timestamp = timestamp.replace(tzinfo=timezone.utc)
                item[key] = timestamp.astimezone(timezone.utc).isoformat()
        yield item


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("database", type=Path)
    args = parser.parse_args()
    with sqlite3.connect(args.database.resolve().as_uri() + "?mode=ro", uri=True) as db:
        db.row_factory = sqlite3.Row
        db.execute("BEGIN")
        json.dump(
            {
                "tasks": list(records(db, "tasks", ("deadline", "timeout_at", "created_at", "finished_at"))),
                "timers": list(records(db, "timers", ("started_at", "created_at"))),
            },
            sys.stdout,
        )
        print()


if __name__ == "__main__":
    main()
