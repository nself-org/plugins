#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Build core nself from origin/main
(cd "$NSELF/cli" && CGO_ENABLED=0 go build -o "$TMPDIR/nself" ./cmd/nself)
CORE_BIN="$TMPDIR/nself"

# Build plugin
(cd "$(dirname "$0")/.." && CGO_ENABLED=0 go build -o "$TMPDIR/nself-functions" ./cmd)
PLUGIN_BIN="$TMPDIR/nself-functions"

# Fixture project
mkdir -p "$TMPDIR/project/functions/myfunc"
echo "test" > "$TMPDIR/project/functions/myfunc/index.js"

# Create a stub docker
mkdir -p "$TMPDIR/bin"
cat << 'DOCKER_EOF' > "$TMPDIR/bin/docker"
#!/usr/bin/env bash
echo "docker $*"
DOCKER_EOF
chmod +x "$TMPDIR/bin/docker"
export PATH="$TMPDIR/bin:$PATH"

# Run stub http server
python3 -c "
import http.server, socketserver
class Handler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'ok')
    def do_POST(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'ok')
    def do_DELETE(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'ok')
socketserver.TCPServer.allow_reuse_address = True
httpd = socketserver.TCPServer(('', 3008), Handler)
httpd.serve_forever()
" > "$TMPDIR/http.log" 2>&1 &
HTTP_PID=$!
trap 'kill $HTTP_PID; rm -rf "$TMPDIR"' EXIT
sleep 1

export FUNCTIONS_PORT=3008
export PROJECT_NAME=myproj

cd "$TMPDIR/project"

check_cmd() {
    local cmd_name="$1"
    shift
    
    set +e
    "$CORE_BIN" functions "$@" > "$TMPDIR/core.out" 2> "$TMPDIR/core.err"
    CORE_RC=$?
    
    "$PLUGIN_BIN" "$@" > "$TMPDIR/plugin.out" 2> "$TMPDIR/plugin.err"
    PLUGIN_RC=$?
    set -e
    
    # Remove Global Flags section
    sed -i '' '/^Global Flags:/,$d' "$TMPDIR/core.out" "$TMPDIR/core.err"
    sed -i '' '/^Global Flags:/,$d' "$TMPDIR/plugin.out" "$TMPDIR/plugin.err"
    
    # Remove Use/help instruction
    sed -i '' '/Use ".* --help" for more information/d' "$TMPDIR/core.out" "$TMPDIR/core.err"
    sed -i '' '/Use ".* --help" for more information/d' "$TMPDIR/plugin.out" "$TMPDIR/plugin.err"
    
    sed -i '' '/INFO ENV resolved for .env cascade/d' "$TMPDIR/core.err"

    # Normalize command prefixes
    sed -i '' 's/nself functions/functions/g' "$TMPDIR/core.out" "$TMPDIR/core.err" "$TMPDIR/plugin.out" "$TMPDIR/plugin.err"
    sed -i '' 's/nself-functions/functions/g' "$TMPDIR/core.out" "$TMPDIR/core.err" "$TMPDIR/plugin.out" "$TMPDIR/plugin.err"
    
    # Ignore trailing empty lines in diff by stripping them
    sed -i '' -e :a -e '/^\n*$/{$d;N;ba' -e '}' "$TMPDIR/core.out" "$TMPDIR/core.err" "$TMPDIR/plugin.out" "$TMPDIR/plugin.err" || true
    
    if [ $CORE_RC -ne $PLUGIN_RC ]; then
        echo "FAIL $cmd_name: exit code diff (core $CORE_RC, plugin $PLUGIN_RC)"
        exit 1
    fi
    if ! cmp -s "$TMPDIR/core.out" "$TMPDIR/plugin.out"; then
        echo "FAIL $cmd_name: stdout diff"
        diff "$TMPDIR/core.out" "$TMPDIR/plugin.out" || true
        exit 1
    fi
    if ! cmp -s "$TMPDIR/core.err" "$TMPDIR/plugin.err"; then
        echo "FAIL $cmd_name: stderr diff"
        diff "$TMPDIR/core.err" "$TMPDIR/plugin.err" || true
        exit 1
    fi
    echo "PASS $cmd_name"
}

check_cmd "help" --help
for sub in deploy invoke list logs delete; do
    check_cmd "$sub help" $sub --help
done

check_cmd "list" list
check_cmd "list json" list --json
check_cmd "deploy" deploy myfunc
check_cmd "invoke" invoke myfunc
check_cmd "logs" logs myfunc
check_cmd "delete" delete myfunc

echo "All parity checks passed"
