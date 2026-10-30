package gates

// Purpose: the review rule of the secrets.gitleaks check (P7-AUTO-17). Every
// entry of the repository's .gitleaksignore must carry a
// `# Reviewed YYYY-MM-DD [by <who>]: <reason>` comment on the same or the
// preceding comment line; an unreviewed entry fails the check with E812, which
// names line numbers only. AddReviewedIgnore is the writer behind
// `nself ci config ignore add`.
// Inputs:  the repository root; for the writer a fingerprint, a reason, an
// optional reviewer and the date.
// Outputs: E812 errors that never quote file content or a fingerprint.
// Constraints: only <root>/.gitleaksignore is read or written, never through a
// symlink; a gate run never adds lines; fingerprints come in the two gitleaks
// forms, commit:path:rule:line (git scan) and path:rule:line (--no-git).

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// IgnoreFile is the gitleaks ignore file, relative to the repository root.
const IgnoreFile = ".gitleaksignore"

// ReviewFormat is the comment every ignore entry needs, as shown to users.
const ReviewFormat = "# Reviewed YYYY-MM-DD [by <who>]: <reason>"

func init() {
	model.Register(model.Code{ID: "E812", Class: "code", Summary: "a .gitleaksignore entry has no review comment", Fix: "Put `" + ReviewFormat + "` on the line before each entry, or add entries with `nself ci config ignore add <fingerprint> --reason <text>`"})
}

// IgnoreError carries E812. Its text holds line numbers only, never content.
type IgnoreError struct{ Code, Message string }

func (e *IgnoreError) Error() string { return e.Code + ": " + e.Message }

func ignoreErr(format string, a ...any) error {
	return &IgnoreError{Code: "E812", Message: fmt.Sprintf(format, a...)}
}

var reviewRe = regexp.MustCompile(`#\s*Reviewed (\d{4}-\d{2}-\d{2})(?: by ([^:#\r\n]*[^:#\s]))?:\s*\S`)

// reviewed reports whether s holds a well-formed review comment with a real date.
func reviewed(s string) bool {
	m := reviewRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	_, err := time.Parse("2006-01-02", m[1])
	return err == nil
}

var (
	ruleRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	commitRe = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

// FingerprintCommit reports whether fp is a valid gitleaks fingerprint and
// whether it carries a commit (the git-scan form).
func FingerprintCommit(fp string) (valid, withCommit bool) {
	if fp == "" || strings.ContainsAny(fp, " \t\r\n#") {
		return false, false
	}
	parts := strings.Split(fp, ":")
	if len(parts) < 3 {
		return false, false
	}
	n := len(parts)
	if line, err := strconv.Atoi(parts[n-1]); err != nil || line < 1 || strconv.Itoa(line) != parts[n-1] {
		return false, false
	}
	if !ruleRe.MatchString(parts[n-2]) {
		return false, false
	}
	withCommit = n >= 4 && commitRe.MatchString(parts[0])
	path := strings.Join(parts[:n-2], ":")
	if withCommit {
		path = strings.Join(parts[1:n-2], ":")
	}
	if path == "" || strings.HasPrefix(path, "/") {
		return false, false
	}
	return true, withCommit
}

// ValidFingerprint reports whether fp has one of the two gitleaks forms.
func ValidFingerprint(fp string) bool { ok, _ := FingerprintCommit(fp); return ok }

// entry splits a line into its fingerprint part (before any '#') and reports
// whether the line is a comment or blank.
func entry(line string) (fp string, comment bool) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", true
	}
	if i := strings.Index(s, "#"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s, false
}

// UnreviewedLines returns the 1-based numbers of the entry lines in r that
// have no review comment on the same or the preceding line. Every line that is
// not blank and not a comment is an entry, recognised fingerprint or not.
func UnreviewedLines(r io.Reader) ([]int, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var out []int
	prevReviewed, n := false, 0
	for sc.Scan() {
		n++
		line := sc.Text()
		_, comment := entry(line)
		if !comment && !reviewed(line) && !prevReviewed {
			out = append(out, n)
		}
		// Only a comment line vouches for the entry after it.
		prevReviewed = comment && reviewed(line)
	}
	return out, sc.Err()
}

// openIgnore opens <root>/.gitleaksignore for reading; a missing file is (nil, nil).
func openIgnore(root string) (*os.File, error) {
	p := filepath.Join(root, IgnoreFile)
	st, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ignoreErr("%s cannot be read", IgnoreFile)
	}
	if !st.Mode().IsRegular() {
		return nil, ignoreErr("%s is not a regular file (a symlink is refused)", IgnoreFile)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, ignoreErr("%s cannot be read", IgnoreFile)
	}
	return f, nil
}

// ReviewIgnoreFile applies the review rule to <root>/.gitleaksignore. A
// missing file passes; an unreviewed entry returns an E812 *IgnoreError.
func ReviewIgnoreFile(root string) error {
	f, err := openIgnore(root)
	if err != nil || f == nil {
		return err
	}
	defer f.Close()
	lines, err := UnreviewedLines(f)
	if err != nil {
		return ignoreErr("%s cannot be read", IgnoreFile)
	}
	if len(lines) == 0 {
		return nil
	}
	nums := make([]string, len(lines))
	for i, l := range lines {
		nums[i] = strconv.Itoa(l)
	}
	word := "line"
	if len(lines) > 1 {
		word = "lines"
	}
	return ignoreErr("%s %s %s has no `%s` comment", IgnoreFile, word, strings.Join(nums, ", "), ReviewFormat)
}

// ApplyIgnoreReview runs the review rule after the gitleaks scan of c. A check
// that errored (tool missing, infrastructure) is left alone; otherwise an
// unreviewed entry turns it into a code failure whose excerpt leads with E812.
func ApplyIgnoreReview(root string, c *model.Check) {
	if c == nil || c.ID != "secrets.gitleaks" || c.Result == "error" || c.Result == "skip" {
		return
	}
	err := ReviewIgnoreFile(root)
	if err == nil {
		return
	}
	c.Result = "fail"
	c.FailureClass = "code"
	if c.Excerpt == "" {
		c.Excerpt = err.Error()
	} else {
		c.Excerpt = err.Error() + "\n" + c.Excerpt
	}
}

// AddReviewedIgnore appends `# Reviewed <date> [by <who>]: <reason>` and fp to
// <root>/.gitleaksignore. It reports false with no error when fp is already an
// entry. Validation of fp, reason and who is the caller's; this only refuses
// what would break the file format. No error text quotes fp.
func AddReviewedIgnore(root, fp, reason, who string, now time.Time) (bool, error) {
	if !ValidFingerprint(fp) {
		return false, ignoreErr("the fingerprint is not in a gitleaks form (commit:path:rule:line or path:rule:line)")
	}
	reason, who = strings.TrimSpace(reason), strings.TrimSpace(who)
	if reason == "" || strings.ContainsAny(reason, "\r\n") {
		return false, ignoreErr("--reason must be one non-empty line")
	}
	if strings.ContainsAny(who, "\r\n:#") {
		return false, ignoreErr("--by must be one line without ':' or '#'")
	}
	var existing []byte
	if f, err := openIgnore(root); err != nil {
		return false, err
	} else if f != nil {
		existing, err = io.ReadAll(f)
		f.Close()
		if err != nil {
			return false, ignoreErr("%s cannot be read", IgnoreFile)
		}
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if got, comment := entry(line); !comment && got == fp {
			return false, nil
		}
	}
	var b bytes.Buffer
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		b.WriteByte('\n')
	}
	b.WriteString("# Reviewed " + now.UTC().Format("2006-01-02"))
	if who != "" {
		b.WriteString(" by " + who)
	}
	b.WriteString(": " + reason + "\n" + fp + "\n")
	f, err := os.OpenFile(filepath.Join(root, IgnoreFile), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return false, ignoreErr("%s cannot be written", IgnoreFile)
	}
	if _, err := f.Write(b.Bytes()); err != nil {
		f.Close()
		return false, ignoreErr("%s cannot be written", IgnoreFile)
	}
	if err := f.Close(); err != nil {
		return false, ignoreErr("%s cannot be written", IgnoreFile)
	}
	return true, nil
}
