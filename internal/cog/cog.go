package cog

type Cog interface {
	Name() string
	Init() error
}
