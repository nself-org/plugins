# Expect-fail page

Block 1 is marked and fails, so it passes. Block 2 is marked but exits 0, so it fails.
Block 3 has the marker separated by a blank line, so the marker does not apply and it fails.

<!-- doc-command: expect-fail -->
```bash
exit 1
```

<!-- doc-command: expect-fail -->
```bash
true
```

<!-- doc-command: expect-fail -->

```bash
exit 5
```
