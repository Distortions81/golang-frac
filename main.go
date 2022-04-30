package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/remeh/sizedwaitgroup"
	"github.com/shirou/gopsutil/cpu"
	"golang.org/x/image/tiff"
)

const (
	chromaMode  = false
	lumaMode    = true
	autoZoom    = true
	startOffset = 9900
	winWidth    = 3840
	winHeight   = 2160
	superSample = 8 //max 255
	maxIters    = 10000
	offX        = 0.747926709975882
	offY        = -0.10785035275635992
	zoomPow     = 100
	zoomDiv     = 10000.0
	escapeVal   = 4.0

	gamma          = 0.25
	reportInterval = 1 * time.Second
)

var (
	palette      [maxIters + 1]uint16
	renderWidth  int    = winWidth
	renderHeight int    = winHeight
	minBright    uint16 = 0xffff
	maxBright    uint16 = 0x0000

	offscreen     *image.RGBA64
	offscreenGray *image.Gray16

	numThreads = runtime.NumCPU()

	curZoom         float64 = 1.0
	zoomInt         int     = startOffset
	lastReported    time.Time
	lastReportedVal float64
	frameCount      int

	cacheSizeKB float64 = 384
	workBlock   int
)

type Game struct {
}

func main() {
	/* Detect logical CPUs */
	var lCPUs int = runtime.NumCPU()
	cdat, cerr := cpu.Counts(false)

	if cerr == nil {
		fmt.Println("Logical CPUs:", cdat)
	} else {
		fmt.Println("Unable to detect logical CPUs.")
	}
	fmt.Println("Threads found:", lCPUs)

	/* Adjust for hyperthreading */
	threadPerCPU := float64(cdat / lCPUs)
	cachePerThread := float64(cacheSizeKB / threadPerCPU)
	if lCPUs < cdat {
		workBlock = int(math.Sqrt((cachePerThread * 1024.0 * 8.0) / 64.0))
	} else {
		workBlock = int(math.Sqrt((cacheSizeKB * 1024.0 * 8.0) / 64.0))
	}
	fmt.Println("Work block size:", workBlock*workBlock, "pixels")
	fmt.Println("Threads per CPU:", threadPerCPU)
	fmt.Println("Cache size:", cacheSizeKB, "KB, per thread:", cachePerThread, "KB")

	if lCPUs < 1 {
		fmt.Println("Invalid number of threads, defaulting to 1.")
		lCPUs = 1
	}

	offscreen = image.NewRGBA64(image.Rect(0, 0, renderWidth, renderHeight))
	offscreenGray = image.NewGray16(image.Rect(0, 0, renderWidth, renderHeight))

	for i := range palette {
		palette[i] = uint16(math.Pow(float64(i)/float64(maxIters), gamma) * float64(0xffff))
	}

	sStep := (float64(zoomInt) / zoomDiv)
	curZoom = (math.Pow(sStep, zoomPow))

	for {
		updateOffscreen()
	}
}

func updateOffscreen() {

	wg := sizedwaitgroup.New(numThreads)

	ss := uint64(superSample * superSample)
	numWorkBlocks := (renderWidth / workBlock) * (renderHeight / workBlock)
	blocksDone := 0

	for xBlock := 0; xBlock < renderWidth/workBlock; xBlock++ {
		for yBlock := 0; yBlock < renderHeight/workBlock; yBlock++ {
			blocksDone++
			percentDone := (float64(blocksDone) / float64(numWorkBlocks) * 100.0)

			if time.Since(lastReported) > reportInterval && lastReportedVal < percentDone {
				lastReported = time.Now()
				fmt.Printf("%0.2f%%\n", percentDone)
				lastReportedVal = percentDone
			}

			wg.Add()
			go func(xBlock, yBlock int) {
				defer wg.Done()

				xStart := xBlock * workBlock
				yStart := yBlock * workBlock

				xEnd := xStart + workBlock
				yEnd := yStart + workBlock

				for x := xStart; x < xEnd; x++ {
					for y := yStart; y < yEnd; y++ {
						var pixel uint64

						for sx := 0; sx < superSample; sx++ {
							for sy := 0; sy < superSample; sy++ {
								ssx := float64(sx) / float64(superSample)
								ssy := float64(sy) / float64(superSample)

								xx := (((float64(x)+ssx)/float64(renderWidth) - 0.5) / (curZoom)) - offX
								yy := (((float64(y)+ssy)/float64(renderWidth) - 0.3) / (curZoom)) - offY

								c := complex(xx, yy) //Rotate
								z := complex(0, 0)

								var it uint16
								for it = 0; it < maxIters; it++ {
									z = z*z + c
									if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
										break
									}
								}

								pixel += uint64(it)
							}
						}
						offscreenGray.SetGray16(x, y, color.Gray16{Y: palette[uint16(pixel/ss)]})

					}
				}
			}(xBlock, yBlock)
		}

	}
	wg.Wait()

	if autoZoom {
		zoomInt = zoomInt + 1
		sStep := (float64(zoomInt) / zoomDiv)
		curZoom = (math.Pow(sStep, zoomPow))

		if chromaMode {

			fileName := fmt.Sprintf("out/color-%v.tif", zoomInt)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, offscreen, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()
		}

		if lumaMode {
			fileName := fmt.Sprintf("out/luma-%v.tif", zoomInt)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, offscreenGray, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()
		}
	}

	frameCount++
	fmt.Println("Completed frame:", frameCount)
}
