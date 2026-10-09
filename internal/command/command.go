package command

type Command interface {
	Name() string
	Args() []string
}
