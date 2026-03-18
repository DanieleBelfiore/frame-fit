package main

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/disintegration/imaging"
	pigo "github.com/esimov/pigo/core"
	"github.com/muesli/smartcrop"
	"github.com/muesli/smartcrop/nfnt"
	"github.com/ncruces/zenity"
)

//go:embed facefinder
var cascadeFile []byte

var faceClassifier *pigo.Pigo

func main() {
	p := pigo.NewPigo()
	var err error
	faceClassifier, err = p.Unpack(cascadeFile)
	if err != nil {
		log.Fatalf("Error: Could not unpack model: %v", err)
	}

	myApp := app.NewWithID("com.danielebelfiore.framefit")
	myWindow := myApp.NewWindow("FrameFit")
	myWindow.Resize(fyne.NewSize(500, 450))

	titleLabel := widget.NewLabelWithStyle("Welcome to FrameFit!", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	inputDirEntry := widget.NewEntry()
	inputDirEntry.SetPlaceHolder("Source folder path...")

	browseInputBtn := widget.NewButton("Browse...", func() {
		path, err := zenity.SelectFile(zenity.Directory())
		if err == nil && path != "" {
			inputDirEntry.SetText(path)
		}
	})

	inputContainer := container.NewBorder(nil, nil, nil, browseInputBtn, inputDirEntry)

	outputDirEntry := widget.NewEntry()
	outputDirEntry.SetPlaceHolder("Destination folder path...")

	browseOutputBtn := widget.NewButton("Browse...", func() {
		path, err := zenity.SelectFile(zenity.Directory())
		if err == nil && path != "" {
			outputDirEntry.SetText(path)
		}
	})

	outputContainer := container.NewBorder(nil, nil, nil, browseOutputBtn, outputDirEntry)

	widthEntry := widget.NewEntry()
	widthEntry.SetText("1280")

	heightEntry := widget.NewEntry()
	heightEntry.SetText("800")

	statusLabel := widget.NewLabel("Waiting... Fill the fields and click Start.")
	statusLabel.Alignment = fyne.TextAlignCenter

	startButton := widget.NewButton("Start Processing", func() {
		inPath := filepath.Clean(strings.TrimSpace(inputDirEntry.Text))
		outPath := filepath.Clean(strings.TrimSpace(outputDirEntry.Text))

		if inPath == "." || inPath == "" {
			statusLabel.SetText("Error: Please provide a valid source folder.")
			return
		}
		if outPath == "." || outPath == "" {
			outPath = filepath.Join(inPath, "Output")
			outputDirEntry.SetText(outPath)
		}

		targetWidth, err := strconv.Atoi(widthEntry.Text)
		if err != nil || targetWidth <= 0 {
			statusLabel.SetText("Error: Invalid width.")
			return
		}
		targetHeight, err := strconv.Atoi(heightEntry.Text)
		if err != nil || targetHeight <= 0 {
			statusLabel.SetText("Error: Invalid height.")
			return
		}

		statusLabel.SetText("Processing... Please wait.")

		go func() {
			err := runProcessing(inPath, outPath, targetWidth, targetHeight)
			if err != nil {
				statusLabel.SetText(fmt.Sprintf("Critical error: %v", err))
			} else {
				statusLabel.SetText("Done! Check the destination folder.")
			}
		}()
	})

	content := container.NewVBox(
		titleLabel,
		widget.NewLabel("Source Folder:"),
		inputContainer,
		widget.NewLabel("Destination Folder:"),
		outputContainer,
		widget.NewLabel("Target Width (px):"),
		widthEntry,
		widget.NewLabel("Target Height (px):"),
		heightEntry,
		widget.NewSeparator(),
		startButton,
		statusLabel,
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

func runProcessing(inPath, outPath string, targetWidth, targetHeight int) error {
	if err := os.MkdirAll(outPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	var successCount, errorCount int32

	err := filepath.WalkDir(inPath, func(currentPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if currentPath == outPath {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(currentPath))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}

		relPath, _ := filepath.Rel(inPath, currentPath)
		finalOutputPath := filepath.Join(outPath, relPath)

		wg.Add(1)
		sem <- struct{}{}

		go func(src string, dest string, width int, height int) {
			defer wg.Done()
			defer func() { <-sem }()

			if processErr := processImage(src, dest, width, height); processErr != nil {
				fmt.Printf("Error processing %s: %v\n", filepath.Base(src), processErr)
				atomic.AddInt32(&errorCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}(currentPath, finalOutputPath, targetWidth, targetHeight)

		return nil
	})

	wg.Wait()
	fmt.Printf("\nDone! Success: %d | Errors: %d\n", successCount, errorCount)
	return err
}

func imageToGrayscalePixels(img image.Image) []uint8 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]uint8, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.GrayModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.Gray)
			pixels[y*width+x] = c.Y
		}
	}
	return pixels
}

func drawRect(img *image.NRGBA, r image.Rectangle, c color.Color, thickness int) {
	for t := 0; t < thickness; t++ {
		rt := image.Rect(r.Min.X+t, r.Min.Y+t, r.Max.X-t, r.Max.Y-t)
		for x := rt.Min.X; x <= rt.Max.X; x++ {
			img.Set(x, rt.Min.Y, c)
			img.Set(x, rt.Max.Y, c)
		}
		for y := rt.Min.Y; y <= rt.Max.Y; y++ {
			img.Set(rt.Min.X, y, c)
			img.Set(rt.Max.X, y, c)
		}
	}
}

func featherEdges(img image.Image, featherSize int) *image.NRGBA {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	dst := image.NewNRGBA(bounds)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)

			distX := x
			if w-1-x < distX {
				distX = w - 1 - x
			}

			distY := y
			if h-1-y < distY {
				distY = h - 1 - y
			}

			minDist := distX
			if distY < minDist {
				minDist = distY
			}

			if minDist < featherSize {
				opacity := float64(minDist) / float64(featherSize)
				c.A = uint8(float64(c.A) * opacity)
			}

			dst.SetNRGBA(bounds.Min.X+x, bounds.Min.Y+y, c)
		}
	}
	return dst
}

func processImage(sourcePath string, destPath string, targetWidth int, targetHeight int) error {
	img, err := imaging.Open(sourcePath, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}

	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()

	isPortrait := imgH >= imgW
	var finalImg image.Image

	if isPortrait {
		bg := imaging.Fill(img, targetWidth, targetHeight, imaging.Center, imaging.Lanczos)
		bg = imaging.Blur(bg, 40.0)
		bg = imaging.AdjustBrightness(bg, -20)

		fg := imaging.Fit(img, targetWidth, targetHeight, imaging.Lanczos)

		fg = featherEdges(fg, 40)

		finalImg = imaging.OverlayCenter(bg, fg, 1.0)

	} else {
		minFaceSize := imgW / 40
		if minFaceSize < 20 {
			minFaceSize = 20
		}

		pixels := imageToGrayscalePixels(img)
		cParams := pigo.CascadeParams{
			MinSize: minFaceSize, MaxSize: 1000,
			ShiftFactor: 0.1, ScaleFactor: 1.1,
			ImageParams: pigo.ImageParams{Pixels: pixels, Rows: imgH, Cols: imgW, Dim: imgW},
		}

		rawFaces := faceClassifier.RunCascade(cParams, 0.0)
		rawFaces = faceClassifier.ClusterDetections(rawFaces, 0.2)

		var confirmedFaces []pigo.Detection
		for _, face := range rawFaces {
			if face.Q >= 5.0 {
				confirmedFaces = append(confirmedFaces, face)
			}
		}

		var cropRect image.Rectangle

		if len(confirmedFaces) > 0 {
			minX, minY, maxX, maxY := imgW, imgH, 0, 0
			for _, face := range confirmedFaces {
				if face.Col < minX {
					minX = face.Col
				}
				if face.Row < minY {
					minY = face.Row
				}
				if face.Col > maxX {
					maxX = face.Col
				}
				if face.Row > maxY {
					maxY = face.Row
				}
			}

			centerX := (minX + maxX) / 2
			centerY := (minY + maxY) / 2

			cropRatio := float64(targetWidth) / float64(targetHeight)
			imgRatio := float64(imgW) / float64(imgH)

			var cropW, cropH int
			if imgRatio > cropRatio {
				cropH = imgH
				cropW = int(float64(cropH) * cropRatio)
			} else {
				cropW = imgW
				cropH = int(float64(cropW) / cropRatio)
			}

			cropX := centerX - cropW/2
			cropY := centerY - int(float64(cropH)*0.33)

			if cropX < 0 {
				cropX = 0
			}
			if cropY < 0 {
				cropY = 0
			}
			if cropX+cropW > imgW {
				cropX = imgW - cropW
			}
			if cropY+cropH > imgH {
				cropY = imgH - cropH
			}

			cropRect = image.Rect(cropX, cropY, cropX+cropW, cropY+cropH)

		} else {
			analyzer := smartcrop.NewAnalyzer(nfnt.NewDefaultResizer())
			topCrop, err := analyzer.FindBestCrop(img, targetWidth, targetHeight)
			if err != nil {
				return err
			}
			cropRect = topCrop
		}

		croppedImg := imaging.Crop(img, cropRect)
		finalImg = imaging.Resize(croppedImg, targetWidth, targetHeight, imaging.Lanczos)
	}

	destDir := filepath.Dir(destPath)
	os.MkdirAll(destDir, os.ModePerm)

	if err := imaging.Save(finalImg, destPath); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	return nil
}