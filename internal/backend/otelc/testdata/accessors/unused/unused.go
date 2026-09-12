package unused

import "context"

func init()                        { panic("unused application package was linked") }
func Process(context.Context, int) {}
