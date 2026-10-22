package backup

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// restoreHome keeps current pins and settings; deny entries from either config remain denied.
func restoreHome(dst, name string, archived []byte) ([]byte, error) {
	current, err := os.ReadFile(dst)
	if errors.Is(err, os.ErrNotExist) {
		return archived, nil
	}
	if err != nil {
		return nil, err
	}
	if name == dirFiles+"/known_hosts" {
		return current, nil
	}
	var old, cur map[string]any
	if err = yaml.Unmarshal(archived, &old); err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(current, &cur); err != nil {
		return nil, err
	}
	oldCI, _ := old["ci"].(map[string]any)
	curCI, _ := cur["ci"].(map[string]any)
	oldDeny, _ := oldCI["never_hosts"].([]any)
	flatDeny, _ := old["ci.never_hosts"].([]any)
	oldDeny = append(oldDeny, flatDeny...)
	if len(oldDeny) == 0 {
		return current, nil
	}
	if cur == nil {
		cur = map[string]any{}
	}
	if curCI == nil {
		curCI = map[string]any{}
		cur["ci"] = curCI
	}
	deny, _ := curCI["never_hosts"].([]any)
	seen := map[any]bool{}
	for _, v := range deny {
		if str, ok := v.(string); ok {
			seen[str] = true
		}
	}
	for _, v := range oldDeny {
		if str, ok := v.(string); ok && !seen[str] {
			deny = append(deny, str)
			seen[str] = true
		}
	}
	curCI["never_hosts"] = deny
	return yaml.Marshal(cur)
}
