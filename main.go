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
	app.Version = `0.4.4`

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
		cli.BoolFlag{
			Name:  `external, x`,
			Usage: `Treat the first argument as a URL to load directly into the window instead of an application bundle`,
		},
		cli.IntFlag{
			Name:  `width, W`,
			Usage: `The width of the window`,
			Value: WindowDefaultWidth,
		},
		cli.IntFlag{
			Name:  `height, H`,
			Usage: `The height of the window`,
			Value: WindowDefaultHeight,
		},
		cli.BoolFlag{
			Name:  `fullscreen, F`,
			Usage: `Make the window fill the entire screen`,
		},
		cli.StringFlag{
			Name:  `title, T`,
			Usage: `The window title`,
		},
	}

	app.Before = func(c *cli.Context) error {
		log.SetLevelString(c.String(`log-level`))
		return nil
	}

	app.Action = func(c *cli.Context) {
		var loadpath = typeutil.OrString(c.Args().First(), `default`)
		var win *Window

		if !c.Bool(`external`) {
			var app, err = FindAppByName(loadpath)
			log.FatalIf(err)

			win = CreateWindow(app)
		} else {
			win = CreateWindowWithConfig(&AppConfig{
				URL: c.Args().First(),
			})
		}

		if v := c.Int(`width`); v > 0 {
			win.Config.Width = v
		}
		if v := c.Int(`height`); v > 0 {
			win.Config.Height = v
		}
		if c.IsSet(`fullscreen`) {
			win.Config.Fullscreen = c.Bool(`fullscreen`)
		}
		if c.IsSet(`title`) {
			win.Config.Name = c.String(`title`)
		}

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
