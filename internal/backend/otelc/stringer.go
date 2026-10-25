package otelc

const stringerGuardSource = `
func otelplanReadStringer(reader func(any) (string, bool), input any) (value string, available bool) {
 defer func() {
  if recover() != nil { value, available = "", false }
 }()
 return reader(input)
}
`

func (renderer *hookRenderer) writeStringerGuard() {
	for _, accessors := range renderer.accessors {
		for _, accessor := range accessors {
			if accessor.Stringer {
				renderer.source.WriteString(stringerGuardSource)

				return
			}
		}
	}
}
