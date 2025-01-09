package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "image/gif" // Import to support GIF format

	"github.com/fogleman/primitive/primitive"
	"github.com/nfnt/resize"
)

var (
	// Global variables can be set via environment variables or a configuration file
	// in a real-world application.
	Background = ""
	Alpha      = 128
	InputSize  = 256
	OutputSize = 1024
	Mode       = 1
	Workers    = runtime.NumCPU()
	Nth        = 1
	Repeat     = 0
	V, VV      bool
)

func errorMessage(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, message, statusCode)
}

func decodeBase64Image(base64String string) (image.Image, string, error) {
	reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(base64String))
	img, format, err := image.Decode(reader)
	if err != nil {
		return nil, "", err
	}
	return img, format, nil
}

func encodeImageToBase64(img image.Image, format string) (string, error) {
	var buf bytes.Buffer
	var err error
	switch format {
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, img, nil)
	case "png":
		err = png.Encode(&buf, img)
	default:
		return "", fmt.Errorf("unsupported image format: %s", format)
	}
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func processImage(w http.ResponseWriter, r *http.Request) {
	// Parse the base64 image from the request body
	var base64Image string
	var imageType string = "png"
	var stepCount int = 200
	if err := r.ParseForm(); err != nil {
		errorMessage(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	base64Image = r.FormValue("image")
	if base64Image == "" {
		errorMessage(w, "Missing 'image' parameter", http.StatusBadRequest)
		return
	}

	if stepCountStr := r.FormValue("stepCount"); stepCountStr != "" {
		stepCount, _ = strconv.Atoi(stepCountStr)
	}
	if imageTypeStr := r.FormValue("imageType"); imageTypeStr != "" {
		imageType = imageTypeStr
	}

	// Decode the base64 image
	img, format, err := decodeBase64Image(base64Image)
	if err != nil {
		errorMessage(w, "Failed to decode base64 image", http.StatusBadRequest)
		return
	}

	// Scale down input image if needed
	if InputSize > 0 {
		img = resize.Thumbnail(uint(InputSize), uint(InputSize), img, resize.Bilinear)
	}

	// Determine background color
	var bg primitive.Color
	if Background == "" {
		bg = primitive.MakeColor(primitive.AverageImageColor(img))
	} else {
		bg = primitive.MakeHexColor(Background)
	}

	// Run the primitive algorithm
	model := primitive.NewModel(img, bg, OutputSize, Workers)
	rand.Seed(time.Now().UTC().UnixNano())
	for i := 0; i < stepCount; i++ { // Example: run for 1000 iterations
		model.Step(primitive.ShapeType(Mode), Alpha, Repeat)
	}

	w.Header().Set("Content-Type", "application/json")

	if imageType == "png" {
		// Encode the result image to base64
		resultBase64, err := encodeImageToBase64(model.Context.Image(), format)
		if err != nil {
			errorMessage(w, "Failed to encode result image", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(fmt.Sprintf(`{"image": "%s"}`, resultBase64)))
	} else if imageType == "svg" {
		var svg = model.SVG()
		w.Write([]byte(fmt.Sprintf(`{"image": "%s"}`, svg)))
	} else if imageType == "gif" {
		frames := model.Frames(0.001)

		g := gif.GIF{}
		for i, src := range frames {
			dst := image.NewPaletted(src.Bounds(), palette.Plan9)
			draw.Draw(dst, dst.Rect, src, image.ZP, draw.Src)
			g.Image = append(g.Image, dst)
			if i == len(frames)-1 {
				g.Delay = append(g.Delay, 250)
			} else {
				g.Delay = append(g.Delay, 50)
			}
		}

		var buf bytes.Buffer
		var err error
		if err = gif.EncodeAll(&buf, &g); err == nil {
			str := base64.StdEncoding.EncodeToString(buf.Bytes())
			w.Write([]byte(fmt.Sprintf(`{"image": "%s"}`, str)))
		} else {
			w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())))
		}
	}

	// Return the result

}

func main() {
	http.HandleFunc("/process", processImage)
	port := flag.String("port", "8080", "HTTP server port")
	flag.Parse()
	log.Printf("Starting server on port %s", *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
