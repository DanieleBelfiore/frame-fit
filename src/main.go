package main

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
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

const (
	maxConcurrentImages = 5
	maxDimension        = 10000
)

func initClassifier() (*pigo.Pigo, error) {
	p := pigo.NewPigo()
	classifier, err := p.Unpack(cascadeFile)
	if err != nil {
		return nil, fmt.Errorf("could not unpack model: %w", err)
	}
	return classifier, nil
}

// uiState groups all widgets that need updating during a processing run.
type uiState struct {
	startButton  *widget.Button
	stopButton   *widget.Button
	statusLabel  *widget.Label
	progressBar  *widget.ProgressBar
	errorLabel   *widget.Label
}

func buildUI(myApp fyne.App, classifier *pigo.Pigo) fyne.Window {
	myWindow := myApp.NewWindow("FrameFit")
	myWindow.Resize(fyne.NewSize(520, 520))

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

	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	errorLabel := widget.NewLabel("")
	errorLabel.Alignment = fyne.TextAlignCenter
	errorLabel.Hide()

	ui := &uiState{
		statusLabel: statusLabel,
		progressBar: progressBar,
		errorLabel:  errorLabel,
	}

	var cancelFn context.CancelFunc

	stopButton := widget.NewButton("Stop", func() {
		if cancelFn != nil {
			cancelFn()
		}
	})
	stopButton.Disable()
	ui.stopButton = stopButton

	var startButton *widget.Button
	startButton = widget.NewButton("Start Processing", func() {
		ui.startButton = startButton
		onStartPressed(ui, classifier, inputDirEntry, outputDirEntry, widthEntry, heightEntry, &cancelFn)
	})
	ui.startButton = startButton

	buttonRow := container.NewGridWithColumns(2, startButton, stopButton)

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
		buttonRow,
		progressBar,
		statusLabel,
		errorLabel,
	)

	myWindow.SetContent(content)
	return myWindow
}

func onStartPressed(
	ui *uiState,
	classifier *pigo.Pigo,
	inputDirEntry, outputDirEntry, widthEntry, heightEntry *widget.Entry,
	cancelFn *context.CancelFunc,
) {
	inPath := filepath.Clean(strings.TrimSpace(inputDirEntry.Text))
	outPath := filepath.Clean(strings.TrimSpace(outputDirEntry.Text))

	if inPath == "." || inPath == "" {
		ui.statusLabel.SetText("Error: Please provide a valid source folder.")
		return
	}
	if outPath == "." || outPath == "" {
		outPath = filepath.Join(inPath, "Output")
		outputDirEntry.SetText(outPath)
	}

	// Prevent overwriting source images.
	absIn, _ := filepath.Abs(inPath)
	absOut, _ := filepath.Abs(outPath)
	if absIn == absOut {
		ui.statusLabel.SetText("Error: Source and destination must be different folders.")
		return
	}

	targetWidth, err := strconv.Atoi(widthEntry.Text)
	if err != nil || targetWidth <= 0 {
		ui.statusLabel.SetText("Error: Invalid width.")
		return
	}
	targetHeight, err := strconv.Atoi(heightEntry.Text)
	if err != nil || targetHeight <= 0 {
		ui.statusLabel.SetText("Error: Invalid height.")
		return
	}
	if targetWidth > maxDimension || targetHeight > maxDimension {
		ui.statusLabel.SetText(fmt.Sprintf("Error: Dimensions exceed maximum (%dpx).", maxDimension))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	*cancelFn = cancel

	ui.startButton.Disable()
	ui.stopButton.Enable()
	ui.errorLabel.Hide()
	ui.progressBar.SetValue(0)
	ui.progressBar.Show()
	ui.statusLabel.SetText("Counting images...")

	go func() {
		defer func() {
			cancel()
			ui.startButton.Enable()
			ui.stopButton.Disable()
			ui.progressBar.Hide()
		}()

		result, runErr := runProcessing(ctx, classifier, inPath, outPath, targetWidth, targetHeight, func(done, total int) {
			if total > 0 {
				ui.progressBar.SetValue(float64(done) / float64(total))
				ui.statusLabel.SetText(fmt.Sprintf("Processing... %d / %d", done, total))
			}
		})

		if ctx.Err() != nil {
			ui.statusLabel.SetText(fmt.Sprintf("Stopped. Processed %d images.", result.success))
			return
		}
		if runErr != nil {
			ui.statusLabel.SetText(fmt.Sprintf("Critical error: %v", runErr))
			return
		}

		ui.statusLabel.SetText(fmt.Sprintf("Done! %d processed, %d errors.", result.success, result.errors))
		if result.errors > 0 {
			ui.errorLabel.SetText(fmt.Sprintf("%d image(s) failed — check the terminal for details.", result.errors))
			ui.errorLabel.Show()
		}
	}()
}

func main() {
	classifier, err := initClassifier()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	myApp := app.NewWithID("com.danielebelfiore.framefit")
	myWindow := buildUI(myApp, classifier)
	myWindow.ShowAndRun()
}

type processingResult struct {
	success int32
	errors  int32
}

func runProcessing(
	ctx context.Context,
	classifier *pigo.Pigo,
	inPath, outPath string,
	targetWidth, targetHeight int,
	onProgress func(done, total int),
) (processingResult, error) {
	if err := os.MkdirAll(outPath, 0o755); err != nil {
		return processingResult{}, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Collect all eligible files first so we can report accurate progress.
	var files []string
	_ = filepath.WalkDir(inPath, func(currentPath string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("Warning: skipping %s: %v\n", currentPath, err)
			return nil
		}
		if d.IsDir() {
			absOut, _ := filepath.Abs(outPath)
			absCur, _ := filepath.Abs(currentPath)
			if absCur == absOut {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(currentPath))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			files = append(files, currentPath)
		}
		return nil
	})

	total := len(files)
	var result processingResult
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentImages)
	var done int32

	for _, src := range files {
		if ctx.Err() != nil {
			break
		}

		relPath, _ := filepath.Rel(inPath, src)
		dest := filepath.Join(outPath, relPath)

		wg.Add(1)
		sem <- struct{}{}

		go func(src, dest string) {
			defer wg.Done()
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			if processErr := processImage(classifier, src, dest, targetWidth, targetHeight); processErr != nil {
				fmt.Printf("Error processing %s: %v\n", filepath.Base(src), processErr)
				atomic.AddInt32(&result.errors, 1)
			} else {
				atomic.AddInt32(&result.success, 1)
			}

			n := int(atomic.AddInt32(&done, 1))
			onProgress(n, total)
		}(src, dest)
	}

	wg.Wait()
	return result, nil
}

func imageToGrayscalePixels(img image.Image) []uint8 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]uint8, width*height)

	// Parallelize over rows for large images.
	numWorkers := runtime.NumCPU()
	rowsPerWorker := height / numWorkers
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		startY := w * rowsPerWorker
		endY := startY + rowsPerWorker
		if w == numWorkers-1 {
			endY = height
		}
		wg.Add(1)
		go func(startY, endY int) {
			defer wg.Done()
			for y := startY; y < endY; y++ {
				for x := 0; x < width; x++ {
					c := color.GrayModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.Gray)
					pixels[y*width+x] = c.Y
				}
			}
		}(startY, endY)
	}
	wg.Wait()
	return pixels
}

func featherEdges(img image.Image, featherSize int) *image.NRGBA {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewNRGBA(bounds)

	numWorkers := runtime.NumCPU()
	rowsPerWorker := h / numWorkers
	var wg sync.WaitGroup

	for worker := 0; worker < numWorkers; worker++ {
		startY := worker * rowsPerWorker
		endY := startY + rowsPerWorker
		if worker == numWorkers-1 {
			endY = h
		}
		wg.Add(1)
		go func(startY, endY int) {
			defer wg.Done()
			for y := startY; y < endY; y++ {
				for x := 0; x < w; x++ {
					c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
					minDist := min(min(x, w-1-x), min(y, h-1-y))
					if minDist < featherSize {
						c.A = uint8(float64(c.A) * float64(minDist) / float64(featherSize))
					}
					dst.SetNRGBA(bounds.Min.X+x, bounds.Min.Y+y, c)
				}
			}
		}(startY, endY)
	}
	wg.Wait()
	return dst
}

func processImage(classifier *pigo.Pigo, sourcePath, destPath string, targetWidth, targetHeight int) error {
	img, err := imaging.Open(sourcePath, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}

	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()

	var finalImg image.Image
	if imgH >= imgW {
		finalImg = processPortrait(img, targetWidth, targetHeight)
	} else {
		finalImg, err = processLandscape(classifier, img, imgW, imgH, targetWidth, targetHeight)
		if err != nil {
			return err
		}
	}

	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create dest dir %s: %w", destDir, err)
	}

	if err := imaging.Save(finalImg, destPath); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	return nil
}

func processPortrait(img image.Image, targetWidth, targetHeight int) image.Image {
	bg := imaging.Fill(img, targetWidth, targetHeight, imaging.Center, imaging.Lanczos)
	bg = imaging.Blur(bg, 40.0)
	bg = imaging.AdjustBrightness(bg, -20)
	fg := featherEdges(imaging.Fit(img, targetWidth, targetHeight, imaging.Lanczos), 40)
	return imaging.OverlayCenter(bg, fg, 1.0)
}

func processLandscape(classifier *pigo.Pigo, img image.Image, imgW, imgH, targetWidth, targetHeight int) (image.Image, error) {
	faces := detectFaces(classifier, img, imgW, imgH)

	var cropRect image.Rectangle
	if len(faces) > 0 {
		cropRect = faceAwareCrop(faces, imgW, imgH, targetWidth, targetHeight)
	} else {
		analyzer := smartcrop.NewAnalyzer(nfnt.NewDefaultResizer())
		topCrop, err := analyzer.FindBestCrop(img, targetWidth, targetHeight)
		if err != nil {
			return nil, err
		}
		cropRect = topCrop
	}

	return imaging.Resize(imaging.Crop(img, cropRect), targetWidth, targetHeight, imaging.Lanczos), nil
}

func detectFaces(classifier *pigo.Pigo, img image.Image, imgW, imgH int) []pigo.Detection {
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

	rawFaces := classifier.RunCascade(cParams, 0.0)
	rawFaces = classifier.ClusterDetections(rawFaces, 0.2)

	var confirmed []pigo.Detection
	for _, face := range rawFaces {
		if face.Q >= 5.0 {
			confirmed = append(confirmed, face)
		}
	}
	return confirmed
}

func faceAwareCrop(faces []pigo.Detection, imgW, imgH, targetWidth, targetHeight int) image.Rectangle {
	minX, minY, maxX, maxY := imgW, imgH, 0, 0
	for _, face := range faces {
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

	cropX := max(0, min(centerX-cropW/2, imgW-cropW))
	cropY := max(0, min(centerY-int(float64(cropH)*0.33), imgH-cropH))

	return image.Rect(cropX, cropY, cropX+cropW, cropY+cropH)
}