# Good fixture

<!-- proven-by: TestMachineDefaults -->
```text
nself ci --detach --json
nself ci nodes projects add vps-a ~/src/project-a
nself ci runs wait run_123 --json
```

## Proven by

| Step | Test |
|---|---|
| Detached run | `TestMachineDefaults` |
