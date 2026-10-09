package state

type State struct {
	HostName string
	Target   string

	Stage      string
	StageIndex int

	HostRecipient string

	SecretsCommit string

	Converged bool
}
