package main

import (
	"fmt"
	"log"
	"math"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten"
	"github.com/hajimehoshi/ebiten/ebitenutil"
	"github.com/hajimehoshi/ebiten/inpututil"
	"github.com/remeh/sizedwaitgroup"
)

const (
	autoZoom     = false
	startOffset  = 48
	superSample  = 0.5
	winWidth     = 1024
	winHeight    = 1024
	renderWidth  = winWidth * superSample
	renderHeight = winHeight * superSample
	maxIters     = 1000
	offX         = -0.34831493420245574
	offY         = 0.606486596104741
	zoomSpeed    = 1
	wheelSpeed   = 0.05
	gamma        = 0.3
)

var curZoom float64 = 1.0
var zoomInt int = startOffset
var sx float64 = 0.0
var sy float64 = 0.0

type Game struct {
	x float64
	y float64
}

func (g *Game) Update() error {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) {
		g.x = 0
		g.y = 0
	} else {
		dx, dy := ebiten.Wheel()
		g.x += dx * wheelSpeed
		g.y += dy * wheelSpeed
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {

	updateOffscreen(offX, offY, curZoom)

	if autoZoom {
		zoomInt = zoomInt + 1
		sStep := float64(zoomInt) / 1000.0
		curZoom = curZoom + (sStep * sStep * float64(zoomSpeed))
	} else {
		curZoom += (g.y) * curZoom * wheelSpeed
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(1.0/superSample, 1.0/superSample)
	op.GeoM.Translate(0, 0)
	// Specify linear filter.
	op.Filter = ebiten.FilterLinear

	screen.DrawImage(offscreen, op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("TPS: %0.2f, frame: %d, zoom: %0.2f, wheel: %0.2f", ebiten.CurrentTPS(), zoomInt-startOffset, curZoom, g.y))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return winWidth, winHeight
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
}
