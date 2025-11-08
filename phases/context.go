package phases

import "sync"

// Context stores arbitrary key/value pairs shared between phases.
type Context struct {
	mu    sync.RWMutex
	store map[string]any
}

// NewContext creates an empty context.
func NewContext() *Context {
	return &Context{
		store: make(map[string]any),
	}
}

// Set assigns a value under the provided key.
func (c *Context) Set(key string, value any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[key] = value
}

// Get retrieves a value, returning false when the key is not present.
func (c *Context) Get(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.store[key]
	return val, ok
}

// MustGet returns the value or panics if the key is missing.
func (c *Context) MustGet(key string) any {
	val, ok := c.Get(key)
	if !ok {
		panic("phases: missing context key " + key)
	}
	return val
}
