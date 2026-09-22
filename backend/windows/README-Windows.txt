MediaStorm for Windows (x64)
================================

Run Start-MediaStorm.cmd. The first launch initializes a private PostgreSQL 16
database and may take a little longer. MediaStorm then listens on port 7777.

To update, stop MediaStorm, extract the new package, and run its
Start-MediaStorm.cmd. Your data stays under LocalAppData.

If you need to recover an account, stop MediaStorm and open Command Prompt in
the extracted package directory. Run Start-MediaStorm.cmd -RecoverMaster for the
admin account, or Start-MediaStorm.cmd -RecoverUsername alice for a regular
account. The generated password is printed in the terminal.

Persistent settings, cache, logs, and database files are stored under:
  %LOCALAPPDATA%\MediaStorm

Replacing or deleting this extracted application directory does not remove your
MediaStorm data. Do not delete the LocalAppData directory unless you intend to
erase the installation.

This initial Windows build is unsigned, so Windows SmartScreen may display a
warning. Download releases only from the official MediaStorm GitHub page and
verify the accompanying SHA-256 file.
