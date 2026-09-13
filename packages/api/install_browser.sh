#!/bin/sh
set -e

echo "INFO: Detected Alpine Linux (Leapcell Go Runtime)"

# Install Chromium and dependencies
echo "INFO: Installing Chromium and fonts..."
apk update
apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    font-noto-emoji \
    wqy-zenhei

# Verify installation
echo "INFO: Verifying Chromium installation..."
if command -v chromium-browser >/dev/null 2>&1; then
    CHROMIUM_PATH=$(command -v chromium-browser)
    echo "INFO: Chromium found at $CHROMIUM_PATH"
elif command -v chromium >/dev/null 2>&1; then
    CHROMIUM_PATH=$(command -v chromium)
    echo "INFO: Chromium found at $CHROMIUM_PATH"
else
    echo "ERROR: Chromium not found!"
    exit 1
fi

# Create a symlink to google-chrome-stable just in case chromedp looks for it specifically
# and to match the previous logic if needed, though chromedp searches for 'chromium' too.
if [ ! -f ./google-chrome-stable ]; then
    ln -s "$CHROMIUM_PATH" ./google-chrome-stable
    echo "INFO: Created symlink ./google-chrome-stable -> $CHROMIUM_PATH"
fi

echo "INFO: Browser installation complete."
