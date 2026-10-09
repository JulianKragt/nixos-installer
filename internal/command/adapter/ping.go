package adapter

type Ping struct {
	Host string
}

func NewPing(host string) Ping {
	return Ping{
		Host: host,
	}
}

func (p Ping) Name() string {
	return "ping"
}

func (p Ping) Args() []string {
	return []string{
		"-c",
		"1",
		p.Host,
	}
}
