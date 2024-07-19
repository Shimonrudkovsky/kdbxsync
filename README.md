# kdbxsync

`kdbxsync` is a small script for synchronizing two KeePass (.kdbx) databases. It is designed for niche, specific use cases, primarily for personal use.

## Features

- Synchronizes two KeePass databases.
- Handles niche synchronization needs for personal use.

## Prerequisites

- Go programming language installed.
- KeePass database files to synchronize.
- macOS
- Google Drive

## Installation

Clone the repository:

```sh
git clone https://github.com/Shimonrudkovsky/kdbxsync.git
cd kdbxsync
go mod download

export KEEPASS_DB_DIRECTORY=/path/to/directory
export KEEPASS_DB_FILE_NAME=test1.kdbx

go run kdbxsync.go
```