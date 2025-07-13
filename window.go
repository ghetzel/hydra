package main

import (
	"embed"
	"fmt"
	"time"
	"unsafe"

	"github.com/ghetzel/go-stockutil/log"
	webview "github.com/webview/webview_go"
)

//go:embed lib/js/*.js
var FS embed.FS

var WindowEmbeddedLibraryPath = `lib/js/hydra.js`
var WindowDefaultWidth = 1024
var WindowDefaultHeight = 768
var AppDefaultURL = `about:blank`
var NativeWindowFactory NativeWindowable

type Windowable interface {
	Navigate(url string) error
	SetTitle(t string) error
	Move(x int, y int) error
	Resize(w int, height int) error
	Run() error
	Destroy() error
	Hide() error
}

type NativeWindowable interface {
	Pointer() unsafe.Pointer
}

type Messagable interface {
	Send(*Message) (*Message, error)
}

type Window struct {
	Config     *AppConfig
	app        *App
	view       webview.WebView
	didInit    bool
	lasterr    error
	fullscreen bool
	w          int
	h          int
}

func CreateWindow(app *App) *Window {
	var win = new(Window)

	if nw := NativeWindowFactory; nw != nil {
		win.view = webview.NewWindow(true, nw.Pointer())
	} else {
		win.view = webview.New(true)
	}

	win.app = app
	win.Config = app.Config

	app.SetWindow(win)

	return win
}

func CreateWindowWithConfig(config *AppConfig) *Window {
	var win = new(Window)

	if nw := NativeWindowFactory; nw != nil {
		win.view = webview.NewWindow(true, nw.Pointer())
	} else {
		win.view = webview.New(true)
	}

	win.Config = config
	return win
}

func (window *Window) init() error {
	if window.view == nil {
		return fmt.Errorf("cannot open window: no view")
	}

	if window.app == nil {
		if window.Config == nil {
			return fmt.Errorf("cannot open window: no app")
		}
	}

	if window.didInit {
		return nil
	} else {
		if jslib, err := FS.ReadFile(WindowEmbeddedLibraryPath); err == nil {
			window.view.Init(string(jslib))
		} else {
			return err
		}

		window.SetTitle(window.Config.Name)
		window.Resize(window.Config.Width, window.Config.Height)

		if window.Config.Fullscreen {
			window.Fullscreen(true)
		}

		window.Navigate(window.Config.URL)
		window.didInit = true
	}

	return nil
}

func (window *Window) Run() error {
	if err := window.init(); err != nil {
		return err
	}

	if window.app != nil {
		go log.FatalIf(window.app.Run(func(a *App) error {
			go a.Config.Services.Run()
			return nil
		}))
	}

	log.Debugf("opening window to URL %q", window.Config.URL)
	window.Navigate(window.Config.URL)
	window.view.Run()
	window.Wait()

	return window.lasterr
}

func (window *Window) Destroy() error {
	window.app.Config.Services.Stop(false)
	window.view.Destroy()
	return nil
}

func (window *Window) Wait() {
	if svc := window.Config.Services; svc != nil {
		svc.Wait()
	}
	log.Debugf("window and all apps stopped")
}

func (window *Window) Navigate(url string) error {
	window.view.Navigate(url)
	return nil
}

func (window *Window) SetTitle(title string) error {
	window.view.SetTitle(title)
	return nil
}

func (window *Window) Move(x int, y int) error {
	return fmt.Errorf("Move: Not Implemented")
}

func (window *Window) Resize(w int, h int) error {
	window.w = w
	window.h = h
	window.view.SetSize(w, h, webview.HintNone)
	return nil
}

func (window *Window) Fullscreen(on bool) error {
	window.fullscreen = on

	if window.fullscreen {
		window.view.SetSize(0, 0, webview.HintMax&webview.HintFixed)
	} else {
		window.Resize(window.w, window.h)
	}

	return nil
}

func (window *Window) Send(req *Message) (*Message, error) {
	var reply = new(Message)
	var err error

	reply.ID = req.ID
	reply.ReceivedAt = req.ReceivedAt
	reply.SentAt = time.Now()

	switch req.ID {
	case `log`:
		var lvl = log.GetLevel(req.Get(`level`, `debug`).String())
		log.Log(lvl, req.Get(`message`, `-- MARK --`).String())

	case `resize`:
		var w = req.Get(`w`, WindowDefaultWidth).NInt()
		var h = req.Get(`h`, WindowDefaultHeight).NInt()
		err = window.Resize(w, h)

	case `move`:
		var x = req.Get(`x`).NInt()
		var y = req.Get(`y`).NInt()
		err = window.Move(x, y)

	case `start`, `stop`, `restart`:
		for _, program := range window.app.Config.Services.Manager.Programs() {
			var e error

			switch req.ID {
			case `start`:
				program.Start()
			case `stop`:
				program.Stop()
			case `restart`:
				program.Restart()
			}

			err = log.AppendError(err, e)
		}

	default:
		err = fmt.Errorf("no such action %q", req.ID)
	}

	return reply, err
}
