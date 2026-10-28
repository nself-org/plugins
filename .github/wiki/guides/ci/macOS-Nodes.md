# macOS node ownership checks

System node bootstrap checks candidate UIDs before allocating an account. Slot
retirement checks the retiring user's numeric UID after stopping its processes
and removing its temporary system folders. Both checks enumerate every mountpoint with the native macOS `mount` command,
including volumes mounted outside `/Volumes`, using `find -xdev`
without following symlinks or limiting directory depth.
The inventory excludes `devfs` and `autofs` pseudo filesystems. Other filesystem
types are scanned, including mounts outside the usual volume directory and paths
containing spaces.

Each check has one 30-second budget across all volumes. An owned file keeps the
UID reserved. A timeout, unreadable mount listing, ambiguous mountpoint, or filesystem error also keeps the account at retirement
and refuses new UID allocation. The root bootstrap process must be able to read
the volumes it scans; grant Full Disk Access to its launcher where macOS privacy
controls require it. Resolve inaccessible volumes or incomplete scans before
retrying. Do not delete a retained account manually to bypass this protection.

The macOS CI leg covers macOS 15 on arm64 and Intel. Both images run the local
capability probe against an executable `xcodebuild` stub that exits with the
unaccepted-license status, and assert that no known Xcode fact is advertised.
The test does not change the machine's license. Intel adds architecture coverage
without depending on the macOS 14 image scheduled for retirement on November 2,
2026 ([retirement notice](https://github.blog/changelog/2026-10-01-github-actions-macos-14-runner-image-retirement/)).
Standard hosted runners are free for this public repository
([runner choices](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job)).
