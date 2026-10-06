#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Build core nself from origin/main in a detached worktree
git -C "$NSELF/cli" fetch origin
git -C "$NSELF/cli" worktree add --detach "$TMPDIR/cli" origin/main
(
    cd "$TMPDIR/cli"
    echo "Core revision: $(git rev-parse HEAD)"
    CGO_ENABLED=0 go build -o "$TMPDIR/nself" ./cmd/nself
)
CORE_BIN="$TMPDIR/nself"

# Build plugin
(cd "$(dirname "$0")/.." && CGO_ENABLED=0 go build -o "$TMPDIR/nself-functions" ./cmd)
PLUGIN_BIN="$TMPDIR/nself-functions"

# Fixture project
mkdir -p "$TMPDIR/project/functions/myfunc"
echo "test" > "$TMPDIR/project/functions/myfunc/index.js"
mkdir -p "$TMPDIR/project/functions/myfunc_err"
echo "test" > "$TMPDIR/project/functions/myfunc_err/index.js"

# Create a stub docker
mkdir -p "$TMPDIR/bin"
cat << 'DOCKER_EOF' > "$TMPDIR/bin/docker"
#!/usr/bin/env bash
echo "docker $*"
# Also record argv to a log file
echo "docker $*" >> "$TMPDIR/docker_requests.log"
DOCKER_EOF
chmod +x "$TMPDIR/bin/docker"
export PATH="$TMPDIR/bin:$PATH"

# Run stub http server
python3 -c "
import http.server, socketserver, hashlib
from urllib.parse import urlparse

class Handler(http.server.BaseHTTPRequestHandler):
    def do_ANY(self):
        parsed = urlparse(self.path)
        path = parsed.path
        query = parsed.query
        
        # Read body
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length)
        body_sha256 = hashlib.sha256(body).hexdigest() if body else 'empty'
        
        # Sorted relevant headers
        headers_to_log = ['content-type', 'authorization']
        sorted_headers = []
        for h in headers_to_log:
            if h in self.headers:
                sorted_headers.append(f'{h}: {self.headers[h]}')
        sorted_headers_str = ';'.join(sorted_headers)
        
        # Log request
        with open('$TMPDIR/http_requests.log', 'a') as f:
            f.write(f'{self.command} {path} {query} {sorted_headers_str} {body_sha256}\n')
        
        # Determine response
        if 'myfunc_err' in path:
            self.send_response(500)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{\"error\": \"internal error\"}')
        else:
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{\"result\": \"success\"}')
            
    def do_GET(self): self.do_ANY()
    def do_POST(self): self.do_ANY()
    def do_DELETE(self): self.do_ANY()
    def do_PUT(self): self.do_ANY()
    def do_PATCH(self): self.do_ANY()

socketserver.TCPServer.allow_reuse_address = True
httpd = socketserver.TCPServer(('', 0), Handler)
with open('$TMPDIR/port.txt', 'w') as f:
    f.write(str(httpd.server_address[1]))
httpd.serve_forever()
" > "$TMPDIR/http.log" 2>&1 &
HTTP_PID=$!
trap 'kill $HTTP_PID; git -C "$NSELF/cli" worktree remove --force "$TMPDIR/cli"; rm -rf "$TMPDIR"' EXIT
sleep 1

export FUNCTIONS_PORT=$(cat "$TMPDIR/port.txt")
export PROJECT_NAME=myproj

cd "$TMPDIR/project"

check_cmd() {
    local cmd_name="$1"
    shift
    
    # clear logs before core
    > "$TMPDIR/http_requests.log"
    > "$TMPDIR/docker_requests.log"
    
    set +e
    "$CORE_BIN" functions "$@" > "$TMPDIR/core.out" 2> "$TMPDIR/core.err"
    CORE_RC=$?
    set -e
    
    # save core logs
    cp "$TMPDIR/http_requests.log" "$TMPDIR/core_http.log"
    cp "$TMPDIR/docker_requests.log" "$TMPDIR/core_docker.log"
    
    # clear logs before plugin
    > "$TMPDIR/http_requests.log"
    > "$TMPDIR/docker_requests.log"
    
    set +e
    "$PLUGIN_BIN" "$@" > "$TMPDIR/plugin.out" 2> "$TMPDIR/plugin.err"
    PLUGIN_RC=$?
    set -e
    
    # save plugin logs
    cp "$TMPDIR/http_requests.log" "$TMPDIR/plugin_http.log"
    cp "$TMPDIR/docker_requests.log" "$TMPDIR/plugin_docker.log"
    
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
    if ! cmp -s "$TMPDIR/core_http.log" "$TMPDIR/plugin_http.log"; then
        echo "FAIL $cmd_name: http request log diff"
        diff "$TMPDIR/core_http.log" "$TMPDIR/plugin_http.log" || true
        exit 1
    fi
    if ! cmp -s "$TMPDIR/core_docker.log" "$TMPDIR/plugin_docker.log"; then
        echo "FAIL $cmd_name: docker request log diff"
        diff "$TMPDIR/core_docker.log" "$TMPDIR/plugin_docker.log" || true
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

check_cmd "invoke err" invoke myfunc_err
check_cmd "invoke method payload" invoke myfunc --method PUT --payload '{"test":1}' --auth secret

# Unreachable case
export FUNCTIONS_PORT=1
check_cmd "list unreachable" list
check_cmd "invoke unreachable" invoke myfunc

echo "All parity checks passed"
