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

	"github.com/PerformLine/go-stockutil/colorutil"
	"github.com/remeh/sizedwaitgroup"
	"golang.org/x/image/tiff"
)

const (
	preIters    = 15
	maxIters    = 800
	chromaMode  = true
	lumaMode    = true
	autoZoom    = true
	startOffset = 9820

	winWidth    = 1280
	winHeight   = 720
	superSample = 64 //max 255

	offX      = 0.747926709975882
	offY      = -0.10785035275635992
	zoomPow   = 100
	zoomDiv   = 10000.0
	escapeVal = 4.0
	zoomAdd   = 1

	gamma            = 0.45454545454545453
	reportInterval   = 1 * time.Second
	workBlock        = 16
	colorDegPerInter = 10
)

var (
	palette      [maxIters + 1]uint16
	renderWidth  int = winWidth
	renderHeight int = winHeight

	offscreen     *image.RGBA
	offscreenGray *image.Gray16

	numThreads = runtime.NumCPU()
	startTime  = time.Now()
	frameTime  = time.Now()

	curZoom         float64 = 1.0
	zoomInt         int     = startOffset
	lastReported    time.Time
	lastReportedVal float64
	frameCount      int
)

type Game struct {
}

func main() {
	offscreen = image.NewRGBA(image.Rect(0, 0, renderWidth, renderHeight))
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

	frameTime = time.Now()

	wg := sizedwaitgroup.New(numThreads)

	ss := uint32(superSample * superSample)
	numWorkBlocks := (renderWidth / workBlock) * (renderHeight / workBlock)
	blocksDone := 0
	lastReportedVal = 0

	for xBlock := 0; xBlock < renderWidth/workBlock; xBlock++ {
		for yBlock := 0; yBlock < renderHeight/workBlock; yBlock++ {
			blocksDone++
			percentDone := (float64(blocksDone) / float64(numWorkBlocks) * 100.0)

			if time.Since(lastReported) > reportInterval && lastReportedVal < percentDone {
				lastReported = time.Now()
				fmt.Printf("%v/%v: %0.2f%%, Work blocks(%d/%d)\n", time.Since(startTime).Round(time.Second).String(), time.Since(frameTime).Round(time.Second).String(), percentDone, blocksDone, numWorkBlocks)
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
						var pixel uint32 = 0
						var r, g, b uint32
						r = 0
						g = 0
						b = 0

						for sx := 0; sx < superSample; sx++ {
							for sy := 0; sy < superSample; sy++ {
								ssx := float64(sx) / float64(superSample)
								ssy := float64(sy) / float64(superSample)

								xx := (((float64(x)+ssx)/float64(renderWidth) - 0.5) / (curZoom)) - offX
								yy := (((float64(y)+ssy)/float64(renderWidth) - 0.3) / (curZoom)) - offY

								pixel += iteratePoint(xx, yy)

								tr, tg, tb := colorutil.HsvToRgb(math.Mod(float64(uint16(pixel/ss)*colorDegPerInter), 360), 1.0, 1.0)
								r += uint32(tr)
								g += uint32(tg)
								b += uint32(tb)
							}
						}

						offscreenGray.SetGray16(x, y, color.Gray16{Y: palette[uint16(pixel/ss)]})

						offscreen.Set(x, y, color.RGBA{uint8(r / ss), uint8(g / ss), uint8(b / ss), 0xFF})

					}
				}
			}(xBlock, yBlock)
		}

	}
	wg.Wait()

	if autoZoom {
		zoomInt = zoomInt + zoomAdd
		sStep := (float64(zoomInt) / zoomDiv)
		curZoom = (math.Pow(sStep, zoomPow))

		if chromaMode {

			fileName := fmt.Sprintf("out/color-%v.tif", frameCount)
			output, err := os.Create(fileName)
			opt := &tiff.Options{Compression: tiff.Deflate, Predictor: true}
			if tiff.Encode(output, offscreen, opt) != nil {
				log.Println("ERROR: Failed to write image:", err)
				os.Exit(1)
			}
			output.Close()
		}

		if lumaMode {
			fileName := fmt.Sprintf("out/luma-%v.tif", frameCount)
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

func iteratePoint(x, y float64) uint32 {

	c := complex(x, y) //Rotate
	z := complex(0, 0)

	var it uint32 = 0

	skip := false
	for i := 0; i < preIters; i++ {
		z = z*z + c
		if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
			skip = true
			break
		}
	}
	var iters uint32 = maxIters - preIters
	if !skip {
		for it = 0; it < iters; it++ {
			z = z*z + c
			if real(z)*real(z)+imag(z)*imag(z) > escapeVal {
				break
			}
		}
	}
	return it

}
