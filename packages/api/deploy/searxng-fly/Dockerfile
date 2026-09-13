# Tiny SearXNG image with the JSON API enabled — for running SearXNG as a
# SECOND machine inside the SAME Fly app as Cinder (sidecar pattern).
#
# The stock searxng/searxng image ships with JSON disabled; this bakes the
# repo's settings.yml (search.formats: [html, json]) into /etc/searxng.
#
# Build with the deploy/ directory as context so the settings file is in
# scope (Docker COPY cannot reach outside the build context):
#
#   cd deploy
#   docker build -f searxng-fly/Dockerfile -t registry.fly.io/searxng-json:latest .
#   fly auth docker && docker push registry.fly.io/searxng-json:latest
FROM searxng/searxng:latest

COPY searxng/settings.yml /etc/searxng/settings.yml