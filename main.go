package main

import (
	"os"
	"os/signal"

	"github.com/ghetzel/cli"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/typeutil"
)

var globalSignal = make(chan os.Signal, 1)

func main() {
	app := cli.NewApp()
	app.Name = `hydra`
	app.Usage = `Standalone browser-based UI application runner`
	app.Version = `0.2.4`

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   `log-level, L`,
			Usage:  `Level of log output verbosity`,
			Value:  `debug`,
			EnvVar: `LOGLEVEL`,
		},
		cli.BoolTFlag{
			Name:   `debug, D`,
			Usage:  `Enable debug mode within the WebView and backend`,
			EnvVar: `HYDRA_DEBUG`,
		},
	}

	app.Before = func(c *cli.Context) error {
		log.SetLevelString(c.String(`log-level`))
		return nil
	}

	app.Action = func(c *cli.Context) {
		var loadpath = typeutil.OrString(c.Args().First(), `default`)
		var app, err = FindAppByName(loadpath)
		log.FatalIf(err)

		var win = CreateWindow(app)

		go handleSignals(func() {
			win.Destroy()
			win.Wait()
		})

		log.FatalIf(win.Run())
	}

	app.Run(os.Args)
}

func handleSignals(handler func()) {
	var signalChan = make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	for _ = range signalChan {
		handler()
		break
	}

	os.Exit(0)
}
