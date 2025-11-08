package phases

import "fmt"

func inputKey(phaseID, inputID string) string {
	return fmt.Sprintf("phase:%s:input:%s", phaseID, inputID)
}

// SetInput stores an input value for a given phase.
func SetInput(ctx *Context, phaseID, inputID string, value any) {
	if ctx == nil {
		return
	}
	ctx.Set(inputKey(phaseID, inputID), value)
}

// GetInput retrieves an input value for a given phase.
func GetInput(ctx *Context, phaseID, inputID string) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	return ctx.Get(inputKey(phaseID, inputID))
}
