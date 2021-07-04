package main

import (
	"fmt"
	"log"
	"math"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten"
	"github.com/remeh/sizedwaitgroup"
)

const (
	renderWidth  = 1024
	renderHeight = 1024
	winWidth     = renderWidth
	winHeight    = renderHeight
	maxIters     = 1000
	offX         = -0.34831493420245574
	offY         = 0.606486596104741
	zoomSpeed    = 1
	gamma        = 0.4
)

var op *ebiten.DrawImageOptions

var curZoom float64 = 1
var zoomInt int = 2

type Game struct{}

func (g *Game) Update() error {
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.DrawImage(offscreen, op)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return renderWidth, renderHeight
}

func main() {
	ebiten.SetWindowSize(winWidth, winHeight)
	ebiten.SetWindowTitle("Mandelbrot (Ebiten Demo)")

	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

var (
	offscreen    *ebiten.Image
	offscreenPix []byte
	palette      [maxIters]int
	numThreads   = runtime.NumCPU()
)

func color(it int) (c byte) {
	if it >= maxIters {
		return 0xff
	}
	l := byte((float64(palette[it]) / float64(maxIters)) * 0xff)
	return l
}

func updateOffscreen(centerX, centerY, size float64) {

	swg := sizedwaitgroup.New(numThreads)
	for j := 0; j < renderHeight; j++ {
		swg.Add()
		go func(j int) {
			defer swg.Done()
			for i := 0; i < renderWidth; i++ {
				x := (float64(j)/float64(renderHeight)-0.5)/size*3.0 + centerX
				y := (float64(i)/float64(renderHeight)-0.5)/size*3.0 + centerY
				c := complex(x, y) //Rotate
				z := complex(0, 0)
				it := 0
				for ; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > 4 {
						break
					}
				}
				a := color(it)
				p := 4 * (i + j*renderWidth)
				offscreenPix[p] = a
				offscreenPix[p+1] = a
				offscreenPix[p+2] = a
				offscreenPix[p+3] = 0xff
			}

		}(j)
	}
	swg.Wait()
	offscreen.ReplacePixels(offscreenPix)
	time.Sleep(time.Millisecond)

}

func init() {

	buf := fmt.Sprintf("Threads found: %x", numThreads)
	fmt.Println(buf)

	fmt.Printf("Allocating image...")
	offscreen = ebiten.NewImage(renderWidth, renderHeight)
	offscreenPix = make([]byte, renderWidth*renderHeight*4)

	op = &ebiten.DrawImageOptions{}
	fmt.Printf("complete!\n")

	fmt.Printf("Building gamma table...")
	swg := sizedwaitgroup.New((numThreads))
	for i := range palette {
		swg.Add()
		go func(i int) {
			defer swg.Done()
			palette[i] = int(math.Pow(float64(i)/float64(maxIters+1), gamma)*maxIters + 1)
			//buf := fmt.Sprintf("%d, ", palette[i])
			//fmt.Print(buf)
		}(i)
	}

	swg.Wait()
	fmt.Printf("complete!\n")

	go func() {
		for {
			updateOffscreen(offX, offY, curZoom)
			zoomInt = zoomInt + 1
			sStep := float64(zoomInt) / 1000.0
			curZoom = curZoom + (sStep * sStep * float64(zoomSpeed))

			//buf := fmt.Sprintf("%f", curZoom)
			//fmt.Println(buf)
		}
	}()
}
