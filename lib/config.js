'use strict';

// Shared configuration + credential storage for the Ecombrain plugin scripts.
// Zero external dependencies — Node built-ins only.

const fs = require('fs');
const os = require('os');
const path = require('path');

// --- Service URLs ----------------------------------------------------------
// Production endpoints. No environment variables are consulted for URLs (so a
// stray/hostile env var can never redirect the bearer token to another host).
const FRONTEND_URL = 'https://ecombrain.sellerplex.com';
const API_URL = 'https://eb-api.sellerplex.com/graphql';
// ---------------------------------------------------------------------------

function stripTrailingSlash(u) {
  return u.replace(/\/+$/, '');
}

function frontendUrl() {
  return stripTrailingSlash(FRONTEND_URL);
}

function apiUrl() {
  return API_URL;
}

function configDir() {
  // Respect XDG_CONFIG_HOME when set, else ~/.config (works on Windows too:
  // os.homedir() → C:\Users\<name>). This only affects the local file location,
  // never where the token is sent.
  const base = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config');
  return path.join(base, 'ecombrain');
}

function credentialsPath() {
  return path.join(configDir(), 'credentials.json');
}

// Restrict a file/dir to the current user only.
//   POSIX  : chmod to the given mode.
//   Windows: mode bits are ignored by NTFS, so reset inherited ACLs and grant
//            full control to only the current user via icacls.
function hardenPermissions(targetPath, { mode, isDir }) {
  if (process.platform === 'win32') {
    const user = process.env.USERNAME;
    if (!user) return; // best effort
    const grant = isDir ? `${user}:(OI)(CI)F` : `${user}:F`;
    try {
      require('child_process').execFileSync(
        'icacls',
        [targetPath, '/inheritance:r', '/grant:r', grant, '/Q'],
        { stdio: 'ignore' }
      );
    } catch (_) {
      /* best effort — e.g. icacls unavailable */
    }
  } else {
    try {
      fs.chmodSync(targetPath, mode);
    } catch (_) {
      /* best effort — e.g. non-POSIX filesystem */
    }
  }
}

function readCredentials() {
  try {
    return JSON.parse(fs.readFileSync(credentialsPath(), 'utf8'));
  } catch (err) {
    return null;
  }
}

function writeCredentials(creds) {
  const dir = configDir();
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  hardenPermissions(dir, { mode: 0o700, isDir: true });

  const file = credentialsPath();
  // Write with restrictive permissions from the start (POSIX), then harden for
  // the current platform (chmod on POSIX, icacls on Windows).
  fs.writeFileSync(file, JSON.stringify(creds, null, 2) + '\n', { mode: 0o600 });
  hardenPermissions(file, { mode: 0o600, isDir: false });
  return file;
}

function getToken() {
  const creds = readCredentials();
  return creds && creds.token ? creds.token : null;
}

function hasToken() {
  return Boolean(getToken());
}

module.exports = {
  frontendUrl,
  apiUrl,
  configDir,
  credentialsPath,
  readCredentials,
  writeCredentials,
  getToken,
  hasToken,
};
