package main

import (
	"github.com/Traliaa/vpn-manager/internal/app"
	"go.uber.org/fx"
)

func main() {
	fx.New(app.Module).Run()
}
