MediaStorm for Windows (x64)
================================

Run Start-MediaStorm.cmd. The first launch initializes a private PostgreSQL 16
database and may take a little longer. MediaStorm then listens on port 7777.

Persistent settings, cache, logs, and database files are stored under:
  %LOCALAPPDATA%\MediaStorm

Replacing or deleting this extracted application directory does not remove your
MediaStorm data. Do not delete the LocalAppData directory unless you intend to
erase the installation.

This initial Windows build is unsigned, so Windows SmartScreen may display a
warning. Download releases only from the official MediaStorm GitHub page and
verify the accompanying SHA-256 file.
