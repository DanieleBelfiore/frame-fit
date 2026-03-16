# FrameFit

FrameFit is a smart, high-performance and privacy-first CLI tool designed to batch-process and perfectly crop your photo library for digital photo frames. 

Instead of dealing with awkward black bars or poorly centered subjects, FrameFit uses offline Artificial Intelligence (Face Detection) and Smart Cropping to ensure your memories look cinematic and perfectly framed.

## ✨ Features

* **🔒 100% Offline & Private:** All image processing and AI face detection happen locally on your machine. Your photos never leave your hard drive.
* **🧠 AI Face Detection & Rule of Thirds:** For horizontal photos, it detects faces and centers the crop around them, applying the photographic "Rule of Thirds" to leave proper headroom.
* **🎯 Smart Crop Fallback:** If no faces are found (e.g., landscapes), it uses an advanced content-aware algorithm to crop the most visually interesting part of the image.
* **🎬 Cinematic Blur for Verticals:** Vertical or square photos aren't awkwardly chopped. They are gracefully placed over a blurred, dimmed version of themselves with smooth, feathered edges (like a TV documentary style).
* **⚡ Blazing Fast Concurrency:** Built with Go, it processes multiple images simultaneously using goroutines.
* **📂 Directory Replication:** Automatically recreates your original folder structure in the output destination.
* **🔄 Auto-Orientation:** Automatically reads EXIF data to fix photos saved sideways by smartphones.

## 🛠️ Prerequisites

1.  **Go:** Make sure you have [Go installed](https://go.dev/doc/install) on your system.
2.  **Cascade Model:** The AI needs the Pigo face detection model to work.
    * Download the `facefinder` file from the [Pigo Repository](https://raw.githubusercontent.com/esimov/pigo/master/cascade/facefinder).
    * Place the `facefinder` file directly in the root folder of this project (if it is not present) (next to `main.go`).

## 🚀 Installation & Setup

1. Clone or download this repository.
2. Open your terminal in the project folder.
3. Initialize the Go module and install the required dependencies:

```bash
go mod init framefit
go get [github.com/disintegration/imaging](https://github.com/disintegration/imaging)
go get [github.com/esimov/pigo/core](https://github.com/esimov/pigo/core)
go get [github.com/muesli/smartcrop](https://github.com/muesli/smartcrop)
go get [github.com/muesli/smartcrop/nfnt](https://github.com/muesli/smartcrop/nfnt)
```

## 🎮 Usage
Run the program from your terminal:
```bash
go run main.go
```

The interactive CLI will guide you through the setup. You can simply press **Enter** to accept the default values shown in the brackets []:
1. **ORIGINAL folder**: The path to the folder containing your source photos.
2. **DESTINATION folder**: Where the processed photos will be saved (Defaults to an **Output** folder inside your source path).
3. **Target WIDTH**: The width of your digital frame in pixels (Defaults to **1280**).
4. **Target HEIGHT**: The height of your digital frame in pixels (Defaults to **800**).

Wait for the process to finish, and check your destination folder for the perfectly framed photos!

## 📦 Dependencies & Credits
FrameFit is built using these awesome open-source libraries:
- [Pigo](https://github.com/esimov/pigo) - Pure Go face detection.
- [Smartcrop](https://github.com/muesli/smartcrop) - Content-aware image cropping.
- [Imaging](https://github.com/disintegration/imaging) - Simple image processing package for Go.