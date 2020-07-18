package rod

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/kit"
)

// Element represents the DOM element
type Element struct {
	ctx           context.Context
	ctxCancel     func()
	timeoutCancel func()

	page *Page

	ObjectID proto.RuntimeRemoteObjectID
}

// FocusE doc is similar to the method Focus
func (el *Element) FocusE() error {
	err := el.ScrollIntoViewE()
	if err != nil {
		return err
	}

	_, err = el.EvalE(true, `this.focus()`, nil)
	return err
}

// ScrollIntoViewE doc is similar to the method ScrollIntoViewIfNeeded
func (el *Element) ScrollIntoViewE() error {
	defer el.tryTrace("scroll into view")()
	el.page.browser.trySlowmotion()

	return proto.DOMScrollIntoViewIfNeeded{ObjectID: el.ObjectID}.Call(el)
}

// ClickE doc is similar to the method Click
func (el *Element) ClickE(button proto.InputMouseButton) error {
	err := el.WaitVisibleE()
	if err != nil {
		return err
	}

	err = el.ScrollIntoViewE()
	if err != nil {
		return err
	}

	box, err := el.BoxE()
	if err != nil {
		return err
	}

	x := box.Left + box.Width/2
	y := box.Top + box.Height/2

	err = el.page.Mouse.MoveE(x, y, 1)
	if err != nil {
		return err
	}

	defer el.tryTrace(string(button) + " click")()

	return el.page.Mouse.ClickE(button)
}

// PressE doc is similar to the method Press
func (el *Element) PressE(key rune) error {
	err := el.WaitVisibleE()
	if err != nil {
		return err
	}

	err = el.FocusE()
	if err != nil {
		return err
	}

	defer el.tryTrace("press " + string(key))()

	return el.page.Keyboard.PressE(key)
}

// SelectTextE doc is similar to the method SelectText
func (el *Element) SelectTextE(regex string) error {
	err := el.FocusE()
	if err != nil {
		return err
	}

	defer el.tryTrace("select text: " + regex)()
	el.page.browser.trySlowmotion()

	js, jsArgs := jsHelper("selectText", Array{regex})
	_, err = el.EvalE(true, js, jsArgs)
	return err
}

// SelectAllTextE doc is similar to the method SelectAllText
func (el *Element) SelectAllTextE() error {
	err := el.FocusE()
	if err != nil {
		return err
	}

	defer el.tryTrace("select all text")()
	el.page.browser.trySlowmotion()

	js, jsArgs := jsHelper("selectAllText", nil)
	_, err = el.EvalE(true, js, jsArgs)
	return err
}

// InputE doc is similar to the method Input
func (el *Element) InputE(text string) error {
	err := el.WaitVisibleE()
	if err != nil {
		return err
	}

	err = el.FocusE()
	if err != nil {
		return err
	}

	defer el.tryTrace("input " + text)()

	err = el.page.Keyboard.InsertTextE(text)
	if err != nil {
		return err
	}

	js, jsArgs := jsHelper("inputEvent", nil)
	_, err = el.EvalE(true, js, jsArgs)
	return err
}

// BlurE is similar to the method Blur
func (el *Element) BlurE() error {
	_, err := el.EvalE(true, "this.blur()", nil)
	return err
}

// SelectE doc is similar to the method Select
func (el *Element) SelectE(selectors []string) error {
	err := el.WaitVisibleE()
	if err != nil {
		return err
	}

	defer el.tryTrace(fmt.Sprintf(
		`select "%s"`,
		strings.Join(selectors, "; ")))()
	el.page.browser.trySlowmotion()

	js, jsArgs := jsHelper("select", Array{selectors})
	_, err = el.EvalE(true, js, jsArgs)
	return err
}

// MatchesE checks if the element can be selected by the css selector
func (el *Element) MatchesE(selector string) (bool, error) {
	res, err := el.EvalE(true, `s => this.matches(s)`, Array{selector})
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

// AttributeE is similar to the method Attribute
func (el *Element) AttributeE(name string) (*string, error) {
	attr, err := el.EvalE(true, "(n) => this.getAttribute(n)", Array{name})
	if err != nil {
		return nil, err
	}

	if attr.Value.Type == gjson.Null {
		return nil, nil
	}

	return &attr.Value.Str, nil
}

// PropertyE is similar to the method Property
func (el *Element) PropertyE(name string) (proto.JSON, error) {
	prop, err := el.EvalE(true, "(n) => this[n]", Array{name})
	if err != nil {
		return proto.JSON{}, err
	}

	return prop.Value, nil
}

// SetFilesE doc is similar to the method SetFiles
func (el *Element) SetFilesE(paths []string) error {
	absPaths := []string{}
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		absPaths = append(absPaths, absPath)
	}

	defer el.tryTrace(fmt.Sprintf("set files: %v", absPaths))
	el.page.browser.trySlowmotion()

	err := proto.DOMSetFileInputFiles{
		Files:    absPaths,
		ObjectID: el.ObjectID,
	}.Call(el)

	return err
}

// DescribeE doc is similar to the method Describe
// But it can choose depth, depth default is 1, -1 to all
// please see https://chromedevtools.github.io/devtools-protocol/tot/DOM/#method-describeNode
func (el *Element) DescribeE(depth int, pierce bool) (*proto.DOMNode, error) {
	var Depth int64
	switch {
	case depth < 0:
		Depth = -1 // -1 to all
	case depth == 0:
		Depth = 1
	default:
		Depth = int64(depth)
	}
	val, err := proto.DOMDescribeNode{ObjectID: el.ObjectID, Depth: Depth, Pierce: pierce}.Call(el)
	if err != nil {
		return nil, err
	}
	return val.Node, nil
}

// ShadowRootE returns the shadow root of this element
func (el *Element) ShadowRootE() (*Element, error) {
	node, err := el.DescribeE(1, false)
	if err != nil {
		return nil, err
	}

	// though now it's an array, w3c changed the spec of it to be a single.
	id := node.ShadowRoots[0].BackendNodeID

	shadowNode, err := proto.DOMResolveNode{BackendNodeID: id}.Call(el)
	if err != nil {
		return nil, err
	}

	return el.page.ElementFromObject(shadowNode.Object.ObjectID), nil
}

// Frame creates a page instance that represents the iframe
func (el *Element) Frame() *Page {
	newPage := *el.page
	newPage.element = el
	newPage.jsHelperObjectID = ""
	newPage.windowObjectID = ""
	return &newPage
}

// TextE doc is similar to the method Text
func (el *Element) TextE() (string, error) {
	js, jsArgs := jsHelper("text", nil)
	str, err := el.EvalE(true, js, jsArgs)
	if err != nil {
		return "", err
	}
	return str.Value.String(), nil
}

// HTMLE doc is similar to the method HTML
func (el *Element) HTMLE() (string, error) {
	str, err := el.EvalE(true, `this.outerHTML`, nil)
	if err != nil {
		return "", err
	}
	return str.Value.String(), nil
}

// VisibleE doc is similar to the method Visible
func (el *Element) VisibleE() (bool, error) {
	js, jsArgs := jsHelper("visible", nil)
	res, err := el.EvalE(true, js, jsArgs)
	if err != nil {
		return false, err
	}
	return res.Value.Bool(), nil
}

// WaitStableE not using requestAnimation here because it can trigger to many checks,
// or miss checks for jQuery css animation.
func (el *Element) WaitStableE(interval time.Duration) error {
	err := el.WaitVisibleE()
	if err != nil {
		return err
	}

	box := el.Box()

	t := time.NewTicker(interval)
	defer t.Stop()

	for range t.C {
		select {
		case <-t.C:
		case <-el.ctx.Done():
			return el.ctx.Err()
		}
		current := el.Box()
		if *box == *current {
			break
		}
		box = current
	}
	return nil
}

// WaitE doc is similar to the method Wait
func (el *Element) WaitE(js string, params Array) error {
	return kit.Retry(el.ctx, Sleeper(), func() (bool, error) {
		res, err := el.EvalE(true, js, params)
		if err != nil {
			return true, err
		}

		if res.Value.Bool() {
			return true, nil
		}

		return false, nil
	})
}

// WaitVisibleE doc is similar to the method WaitVisible
func (el *Element) WaitVisibleE() error {
	js, jsArgs := jsHelper("visible", nil)
	return el.WaitE(js, jsArgs)
}

// WaitInvisibleE doc is similar to the method WaitInvisible
func (el *Element) WaitInvisibleE() error {
	js, jsArgs := jsHelper("invisible", nil)
	return el.WaitE(js, jsArgs)
}

// Box represents the element bounding rect
type Box struct {
	Top    float64 `json:"top"`
	Left   float64 `json:"left"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// BoxE doc is similar to the method Box
func (el *Element) BoxE() (*Box, error) {
	res, err := proto.DOMGetBoxModel{ObjectID: el.ObjectID}.Call(el)
	if err != nil {
		return nil, err
	}
	return &Box{
		Top:    res.Model.Content[1],
		Left:   res.Model.Content[0],
		Width:  res.Model.Content[2] - res.Model.Content[0],
		Height: res.Model.Content[7] - res.Model.Content[1],
	}, nil
}

// CanvasToImageE get image data of a canvas.
// The default format is image/png.
// The default quality is 0.92.
// doc: https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/toDataURL
func (el *Element) CanvasToImageE(format string, quality float64) ([]byte, error) {
	res, err := el.EvalE(true,
		`(format, quality) => this.toDataURL(format, quality)`,
		Array{format, quality})
	if err != nil {
		return nil, err
	}

	_, bin := parseDataURI(res.Value.Str)
	if err != nil {
		return nil, err
	}

	return bin, nil
}

// ResourceE doc is similar to the method Resource
func (el *Element) ResourceE() ([]byte, error) {
	js, jsArgs := jsHelper("resource", nil)
	src, err := el.EvalE(true, js, jsArgs)
	if err != nil {
		return nil, err
	}

	defer el.page.EnableDomain(&proto.PageEnable{})()

	frameID, err := el.page.frameID()
	if err != nil {
		return nil, err
	}

	res, err := proto.PageGetResourceContent{
		FrameID: frameID,
		URL:     src.Value.String(),
	}.Call(el)
	if err != nil {
		return nil, err
	}

	data := res.Content

	var bin []byte
	if res.Base64Encoded {
		bin, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, err
		}
	} else {
		bin = []byte(data)
	}

	return bin, nil
}

// ScreenshotE of the area of the element
func (el *Element) ScreenshotE(format proto.PageCaptureScreenshotFormat, quality int) ([]byte, error) {
	err := el.WaitVisibleE()
	if err != nil {
		return nil, err
	}

	err = el.ScrollIntoViewE()
	if err != nil {
		return nil, err
	}

	box, err := el.BoxE()
	if err != nil {
		return nil, err
	}

	opts := &proto.PageCaptureScreenshot{
		Format: format,
		Clip: &proto.PageViewport{
			X:      box.Left,
			Y:      box.Top,
			Width:  box.Width,
			Height: box.Height,
			Scale:  1,
		},
	}

	if quality > -1 {
		opts.Quality = int64(quality)
	}

	return el.page.Root().ScreenshotE(false, opts)
}

// ReleaseE doc is similar to the method Release
func (el *Element) ReleaseE() error {
	err := el.page.Context(el.ctx, el.ctxCancel).ReleaseE(el.ObjectID)
	if err != nil {
		return err
	}

	el.ctxCancel()
	return nil
}

// CallContext parameters for proto
func (el *Element) CallContext() (context.Context, proto.Client, string) {
	return el.ctx, el.page.browser, string(el.page.SessionID)
}

// EvalE doc is similar to the method Eval
func (el *Element) EvalE(byValue bool, js string, params Array) (*proto.RuntimeRemoteObject, error) {
	return el.page.Context(el.ctx, el.ctxCancel).EvalE(byValue, el.ObjectID, js, params)
}

func (el *Element) ensureParentPage(nodeID proto.DOMNodeID, objID proto.RuntimeRemoteObjectID) error {
	has, err := el.page.hasElement(objID)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	// DFS for the iframe that holds the element
	var walk func(page *Page) error
	walk = func(page *Page) error {
		list, err := page.ElementsE("", "iframe")
		if err != nil {
			return err
		}

		for _, f := range list {
			p := f.Frame()

			objID, err := p.resolveNode(nodeID)
			if err != nil {
				return err
			}
			if objID != "" {
				el.page = p
				el.ObjectID = objID
				return io.EOF
			}

			err = walk(p)

			err = f.ReleaseE()
			if err != nil {
				return err
			}

			if err != nil {
				return err
			}
		}
		return nil
	}

	err = walk(el.page)
	if err == io.EOF {
		return nil
	}
	return err
}
