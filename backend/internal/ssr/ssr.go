// Package ssr renders React pages in-process by evaluating the frontend's
// server bundle with goja, a pure-Go JavaScript engine.
package ssr

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// Result is what the bundle's global render() returns.
type Result struct {
	HTML        string `json:"html"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Renderer is safe for concurrent use. A goja runtime is single-threaded, so
// each render borrows one from a pool; the bundle is compiled only once.
type Renderer struct {
	program *goja.Program
	pool    sync.Pool
}

func New(bundle []byte) (*Renderer, error) {
	program, err := goja.Compile("server.js", polyfills+string(bundle), true)
	if err != nil {
		return nil, fmt.Errorf("compile server bundle: %w", err)
	}
	r := &Renderer{program: program}
	// Load one runtime eagerly so a broken bundle fails at startup, not on
	// the first request.
	vm, err := r.newRuntime()
	if err != nil {
		return nil, err
	}
	r.pool.Put(vm)
	return r, nil
}

type runtime struct {
	vm     *goja.Runtime
	render goja.Callable
}

func (r *Renderer) newRuntime() (*runtime, error) {
	vm := goja.New()
	if _, err := vm.RunProgram(r.program); err != nil {
		return nil, fmt.Errorf("run server bundle: %w", err)
	}
	render, ok := goja.AssertFunction(vm.Get("render"))
	if !ok {
		return nil, fmt.Errorf("server bundle does not define a global render function")
	}
	return &runtime{vm: vm, render: render}, nil
}

// Render passes page to the bundle's render() as JSON and returns the markup.
func (r *Renderer) Render(page any) (Result, error) {
	rt, _ := r.pool.Get().(*runtime)
	if rt == nil {
		var err error
		if rt, err = r.newRuntime(); err != nil {
			return Result{}, err
		}
	}

	pageJSON, err := json.Marshal(page)
	if err != nil {
		r.pool.Put(rt)
		return Result{}, err
	}
	out, err := rt.render(goja.Undefined(), rt.vm.ToValue(string(pageJSON)))
	if err != nil {
		// Don't reuse a runtime whose state may be corrupted by the exception.
		return Result{}, fmt.Errorf("render: %w", err)
	}
	r.pool.Put(rt)

	var res Result
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		return Result{}, fmt.Errorf("decode render output: %w", err)
	}
	return res, nil
}

// Minimal browser globals React's server renderer touches. goja provides
// the ECMAScript standard library only.
const polyfills = `
var self = globalThis;
if (typeof queueMicrotask === "undefined") {
  var queueMicrotask = function (cb) { Promise.resolve().then(cb); };
}
if (typeof setTimeout === "undefined") {
  var setTimeout = function (cb) { queueMicrotask(cb); return 0; };
  var clearTimeout = function () {};
}
if (typeof TextEncoder === "undefined") {
  var TextEncoder = function () {};
  TextEncoder.prototype.encode = function (str) {
    var utf8 = unescape(encodeURIComponent(str));
    var out = new Uint8Array(utf8.length);
    for (var i = 0; i < utf8.length; i++) out[i] = utf8.charCodeAt(i);
    return out;
  };
}
`
