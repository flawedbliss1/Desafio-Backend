package main

import (
	"go.uber.org/fx"
)

func main() {

	fx.New(

		fx.Module(
			"config",
			fx.Provide(newHTTPClient),
			fx.Provide(NewAuthConfig),
		),

		fx.Module(
			"database",
			fx.Provide(newPool),
		),

		fx.Module(
			"http",
			fx.Provide(newMux),
			fx.Invoke(registerHTTPServer),
		),
	).Run()
}
