package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Config ogp.app config
type Config struct {
	BaseURL            string  `toml:"base_url"`
	APIServerBind      string  `toml:"api_server_bind"`
	APIServerPort      string  `toml:"api_server_port"`
	TLS                bool    `toml:"tls"`
	KoruriBoldFontPath string  `toml:"koruri_bold_font_path"`
	DefaultImageWidth  int     `toml:"default_image_width"`
	DefaultImageHeight int     `toml:"default_image_height"`
	DefaultFontSize    float64 `toml:"default_font_size"`
	ServerCertPath     string  `toml:"server_cert_path"`
	ServerKeyPath      string  `toml:"server_key_path"`
}

// App ogp.app
type App struct {
	Config        *Config
	KoruriBold    *truetype.Font
	OgpPagePath   string
	IndexPagePath string
	OgpPageTmpl   *template.Template
	IndexPageTmpl string
}

// NewConfig create app config
func NewConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to open path %v: %w", path, err)
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("Failed to read file %v: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal toml data: %w", err)
	}
	return &cfg, nil
}

// NewApp create app
func NewApp(cfg *Config) (*App, error) {
	fontBytes, err := ioutil.ReadFile(cfg.KoruriBoldFontPath)
	if err != nil {
		return nil, err
	}
	ft, err := freetype.ParseFont(fontBytes)
	if err != nil {
		return nil, err
	}
	pf, err := os.Open(path.Join("client", "dist", "p.html"))
	if err != nil {
		return nil, err
	}
	defer pf.Close()

	pbuf, err := ioutil.ReadAll(pf)
	if err != nil {
		return nil, err
	}
	ogpPageTmpl, err := template.New("page").Parse(string(pbuf))
	if err != nil {
		return nil, err
	}
	idxf, err := os.Open(path.Join("client", "dist", "index.html"))
	if err != nil {
		return nil, err
	}
	defer idxf.Close()

	idxbuf, err := ioutil.ReadAll(idxf)
	if err != nil {
		return nil, err
	}

	return &App{
		Config:        cfg,
		KoruriBold:    ft,
		OgpPageTmpl:   ogpPageTmpl,
		IndexPageTmpl: string(idxbuf),
	}, nil
}

func createImage(width, height int, fontsize float64, ft *truetype.Font, text, out string) error {
	logger.Info().Str("words", text).Send()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.White, image.ZP, draw.Src)

	opt := truetype.Options{
		Size: fontsize,
	}
	face := truetype.NewFace(ft, &opt)
	dr := &font.Drawer{
		Dst:  img,
		Src:  image.Black,
		Face: face,
		Dot:  fixed.Point26_6{},
	}
	x := (fixed.I(width) - dr.MeasureString(text)) / 2
	dr.Dot.X = x
	y := (height + int(fontsize)/2) / 2
	dr.Dot.Y = fixed.I(y)

	dr.DrawString(text)

	outfile, err := os.Create(out)
	if err != nil {
		return err
	}
	defer outfile.Close()

	if err := png.Encode(outfile, img); err != nil {
		return err
	}
	return nil
}

// OgpPage display ogp page
func (app *App) OgpPage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	data := map[string]string{
		"id":      id,
		"file":    fmt.Sprintf("%s.png", id),
		"baseURL": app.Config.BaseURL,
	}
	w.WriteHeader(http.StatusOK)
	if err := app.OgpPageTmpl.Execute(w, data); err != nil {
		return
	}
	return
}

// IndexPage display index page
func (app *App) IndexPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, app.IndexPageTmpl)
	return
}

type createImageReq struct {
	Words string `json:"words"`
}

// CreateImage create ogp image API
func (app *App) CreateImage(w http.ResponseWriter, r *http.Request) {
	var d createImageReq
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		logger.Error().Msgf("decode failed: %s", err)
		return
	}
	words := d.Words
	id := uuid.New()
	filename := fmt.Sprintf("%s.png", id.String())
	filepath := path.Join("data", filename)
	wi, he, fs := app.Config.DefaultImageWidth, app.Config.DefaultImageHeight, app.Config.DefaultFontSize
	if err := createImage(wi, he, fs, app.KoruriBold, words, filepath); err != nil {
		logger.Error().Msgf("create image failed: %s", err)
		return
	}
	w.WriteHeader(http.StatusOK)
	data := map[string]string{
		"words":   words,
		"file":    filename,
		"id":      id.String(),
		"baseURL": app.Config.BaseURL,
	}
	w.Header().Set("Content-type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(data); err != nil {
		logger.Printf("encode failed: %s", err)
		return
	}
	return
}
