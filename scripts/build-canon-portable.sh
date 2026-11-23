#!/usr/bin/env bash
# Build locally from the vaulted Canon DLLs; never embed a vault password.
set -euo pipefail
if [ "$#" -ne 2 ]; then
  echo "Usage: $0 /path/to/ansible-library /path/to/vault-password-file" >&2
  exit 2
fi
repo_root=$(cd "$(dirname "$0")/.." && pwd)
ansible_root=$(cd "$1" && pwd)
password_file=$2
case "$password_file" in /*) ;; *) password_file="$(pwd)/$password_file" ;; esac
cd "$repo_root"
sdk_dir=internal/camera/edsdk/embedded
if [ -e "$sdk_dir" ]; then echo "Refusing to overwrite $sdk_dir" >&2; exit 1; fi
trap 'rm -rf "$sdk_dir"' EXIT
mkdir -p "$sdk_dir" dist
for dll in EDSDK.dll EdsImage.dll; do
  ansible-vault decrypt --vault-password-file "$password_file" --output "$sdk_dir/$dll" "$ansible_root/roles/canon_edsdk/files/$dll.vault"
done
printf '%s  %s\n' \
  2504ecdad15dbd4c141de3c43ea8c4e1b38d34e462a81b2d1b16b69cbbb11c98 "$sdk_dir/EDSDK.dll" \
  714a226306bdcda32639c4ca97957c4ef390f1c61e3aac20cc5f61b992ebeb40 "$sdk_dir/EdsImage.dll" | sha256sum -c -
if [ ! -f cmd/photobooth/frontend_dist/index.html ]; then echo 'Build the frontend first (cd frontend && npm ci && npm run build).' >&2; exit 1; fi
tags=embedcanon
if [ -f internal/video/embedded/ffmpeg.exe.gz ] && [ -f internal/video/embedded/FFMPEG-LICENSE.txt ]; then
  tags=embedcanon,embedffmpeg
else
  echo 'Note: FFmpeg is not staged; video requires a separate FFmpeg installation.' >&2
fi
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags "$tags" -ldflags '-H windowsgui -s -w' -o dist/ApexVoidPhotobooth-canon.exe ./cmd/photobooth
printf 'Built %s/dist/ApexVoidPhotobooth-canon.exe\n' "$repo_root"
