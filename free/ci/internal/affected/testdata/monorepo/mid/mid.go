package mid

import "example.test/leaf"

// Mid depends on leaf.
func Mid() int { return leaf.Leaf() + 1 }
