#!/bin/sh
set -e

URL="https://github.com/noxymoto/t2f/releases/download/linux-x64-pre/t2f"

echo "Downloading t2f compiler..."
curl -fsSL "$URL" -o /tmp/t2f

sudo cp /tmp/t2f /usr/local/bin/t2f
sudo chmod +x /usr/local/bin/t2f

rm /tmp/t2f

echo "t2f installed to /usr/local/bin/t2f"
