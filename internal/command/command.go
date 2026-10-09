package command

type Command interface {
	Name() string
	Args() []string
}

type Cmd struct {
	name string
	args []string
}

func (c Cmd) Name() string   { return c.name }
func (c Cmd) Args() []string { return c.args }

func New(name string, args ...string) Cmd {
	return Cmd{name: name, args: args}
}
