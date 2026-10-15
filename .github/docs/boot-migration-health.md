# Boot migration health

The boot migration helper uses the HTTP request context with a five-second
timeout for migration status reads. Request cancellation and earlier deadlines
take precedence. A failed status read returns HTTP 503 and counts of `-1`;
successful reads return the applied and expected counts. Call the helper before
committing the response headers.

The `scripts/codemods/bootmig` template carries this behavior into newly wired
plugins. Automatic wiring requires a named request parameter in the enclosing
handler; a handler without an accessible request needs manual wiring.

## GitHub runner lifecycle discovery

Inspection of CLI revision `75d0190c960939429dc0b9b47d372036afe56699`
(`internal/plugin/installer_locked.go`, `installer_finish.go`, and
`installer_remove_update.go`) found no filename discovery or execution of
`install.sh` or `uninstall.sh`. The package has no readers for the old
`postInstall` or `preUninstall` manifest hooks. The GitHub runner's v2 manifest
does not declare those hooks or legacy config defaults.

The presence of the runner scripts therefore does not establish that CLI
installation registers a host runner or that removal unregisters it. Restoring
that behavior needs a separate CLI lifecycle contract decision; this timeout
fix does not introduce a hook API or execute the scripts.
