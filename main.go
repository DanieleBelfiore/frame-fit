package main

import (
	"bufio"
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

	"github.com/disintegration/imaging"
	pigo "github.com/esimov/pigo/core"
	"github.com/muesli/smartcrop"
	"github.com/muesli/smartcrop/nfnt"
)

var faceClassifier *pigo.Pigo

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n")
	fmt.Println("========================================")
	fmt.Println("          Welcome To FrameFit!          ")
	fmt.Println("========================================")
	fmt.Println("\n")

	cascadeFile, err := os.ReadFile("facefinder")
	if err != nil {
		log.Fatalf("❌ Error: Could not find 'facefinder' file.")
	}

	p := pigo.NewPigo()
	faceClassifier, err = p.Unpack(cascadeFile)
	if err != nil {
		log.Fatalf("❌ Error: Could not unpack model: %v", err)
	}

	inputDir := getUserInput(reader, "👉 Enter ORIGINAL folder", "")
	if inputDir == "" { log.Fatalf("❌ Error: You must provide an input directory.") }
	inPath := filepath.Clean(inputDir)

	defaultOutputDir := filepath.Join(inPath, "Output")
	outputDir := getUserInput(reader, "👉 Enter DESTINATION folder", defaultOutputDir)
	outPath := filepath.Clean(outputDir)

	widthStr := getUserInput(reader, "👉 Enter target WIDTH in px", "1280")
	heightStr := getUserInput(reader, "👉 Enter target HEIGHT in px", "800")

	targetWidth, err := strconv.Atoi(widthStr)
	if err != nil || targetWidth <= 0 { log.Fatalf("❌ Invalid width.") }
	targetHeight, err := strconv.Atoi(heightStr)
	if err != nil || targetHeight <= 0 { log.Fatalf("❌ Invalid height.") }

	if err := os.MkdirAll(outPath, os.ModePerm); err != nil {
		log.Fatalf("❌ Error creating output directory: %v", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	var successCount, errorCount int32

	fmt.Printf("\n🚀 Starting...\n\nSource: %s\nTarget: %s\n", inPath, outPath)

	err = filepath.WalkDir(inPath, func(currentPath string, d os.DirEntry, err error) error {
		if err != nil { return err }

		if d.IsDir() {
			if currentPath == outPath { return filepath.SkipDir }
			return nil
		}

		ext := strings.ToLower(filepath.Ext(currentPath))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" { return nil }

		relPath, _ := filepath.Rel(inPath, currentPath)
		finalOutputPath := filepath.Join(outPath, relPath)
		
		wg.Add(1)
		sem <- struct{}{}

		go func(src string, dest string, width int, height int) {
			defer wg.Done()
			defer func() { <-sem }()

			if processErr := processImage(src, dest, width, height); processErr != nil {
				fmt.Printf("❌ Error %s: %v\n", filepath.Base(src), processErr)
				atomic.AddInt32(&errorCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}(currentPath, finalOutputPath, targetWidth, targetHeight)

		return nil
	})

	wg.Wait()
	fmt.Printf("\n🎉 Done! Success: %d | Errors: %d\n", successCount, errorCount)
}

func getUserInput(reader *bufio.Reader, prompt string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" { return defaultValue }
	return input
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
			if w-1-x < distX { distX = w - 1 - x }
			
			distY := y
			if h-1-y < distY { distY = h - 1 - y }
			
			minDist := distX
			if distY < minDist { minDist = distY }

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
	if err != nil { return fmt.Errorf("failed to open image: %w", err) }

	imgW, imgH := img.Bounds().Dx(), img.Bounds().Dy()
	//fileName := filepath.Base(sourcePath)

	isPortrait := imgH >= imgW
	var finalImg image.Image

	if isPortrait {
		//fmt.Printf("📱 Vertical photo detected: %s (Applying Cinematic Blur with Soft Edges)\n", fileName)

		bg := imaging.Fill(img, targetWidth, targetHeight, imaging.Center, imaging.Lanczos)
		bg = imaging.Blur(bg, 40.0)
		bg = imaging.AdjustBrightness(bg, -20)

		fg := imaging.Fit(img, targetWidth, targetHeight, imaging.Lanczos)
		
		fg = featherEdges(fg, 40)

		finalImg = imaging.OverlayCenter(bg, fg, 1.0)

	} else {
		minFaceSize := imgW / 40 
		if minFaceSize < 20 { minFaceSize = 20 }

		pixels := imageToGrayscalePixels(img)
		cParams := pigo.CascadeParams{
			MinSize: minFaceSize,  MaxSize: 1000,
			ShiftFactor: 0.1, ScaleFactor: 1.1,
			ImageParams: pigo.ImageParams{ Pixels: pixels, Rows: imgH, Cols: imgW, Dim: imgW },
		}

		rawFaces := faceClassifier.RunCascade(cParams, 0.0)
		rawFaces = faceClassifier.ClusterDetections(rawFaces, 0.2)

		var confirmedFaces []pigo.Detection
		for _, face := range rawFaces {
			if face.Q >= 5.0 { confirmedFaces = append(confirmedFaces, face) }
		}

		var cropRect image.Rectangle

		if len(confirmedFaces) > 0 {
			//fmt.Printf("🕵️  Found %d real face(s) in: %s\n", len(confirmedFaces), fileName)
			minX, minY, maxX, maxY := imgW, imgH, 0, 0
			for _, face := range confirmedFaces {
				if face.Col < minX { minX = face.Col }
				if face.Row < minY { minY = face.Row }
				if face.Col > maxX { maxX = face.Col }
				if face.Row > maxY { maxY = face.Row }
			}

			centerX := (minX + maxX) / 2
			centerY := (minY + maxY) / 2

			cropRatio := float64(targetWidth) / float64(targetHeight)
			imgRatio := float64(imgW) / float64(imgH)

			var cropW, cropH int
			if imgRatio > cropRatio {
				cropH = imgH; cropW = int(float64(cropH) * cropRatio)
			} else {
				cropW = imgW; cropH = int(float64(cropW) / cropRatio)
			}

			cropX := centerX - cropW/2
			cropY := centerY - int(float64(cropH)*0.33) 

			if cropX < 0 { cropX = 0 }
			if cropY < 0 { cropY = 0 }
			if cropX+cropW > imgW { cropX = imgW - cropW }
			if cropY+cropH > imgH { cropY = imgH - cropH }

			cropRect = image.Rect(cropX, cropY, cropX+cropW, cropY+cropH)

		} else {
			//fmt.Printf("🖼️  No faces found in %s, using smart crop...\n", fileName)
			analyzer := smartcrop.NewAnalyzer(nfnt.NewDefaultResizer())
			topCrop, err := analyzer.FindBestCrop(img, targetWidth, targetHeight)
			if err != nil { return err }
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