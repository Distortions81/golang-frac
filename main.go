package main

import (
	"fmt"
	"log"
	"math"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten"
	"github.com/hajimehoshi/ebiten/ebitenutil"
	"github.com/remeh/sizedwaitgroup"
)

const (
	startOffset  = 32
	multiSample  = 4
	winWidth     = 1024
	winHeight    = 1024
	renderWidth  = winWidth * multiSample
	renderHeight = winHeight * multiSample
	maxIters     = 1000
	offX         = -0.34831493420245574
	offY         = 0.606486596104741
	zoomSpeed    = 1
	gamma        = 0.4
)

var curZoom float64 = 1
var zoomInt int = startOffset

type viewport struct {
	x16 int
	y16 int
}

func (p *viewport) Move() {
}

func (p *viewport) Position() (int, int) {
	return p.x16, p.y16
}

type Game struct {
	viewport viewport
}

func (g *Game) Update() error {
	g.viewport.Move()
	return nil
}
func (g *Game) Draw(screen *ebiten.Image) {

	updateOffscreen(offX, offY, curZoom)
	zoomInt = zoomInt + 1
	sStep := float64(zoomInt) / 1000.0
	curZoom = curZoom + (sStep * sStep * float64(zoomSpeed))

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(1.0/multiSample, 1.0/multiSample)
	op.GeoM.Translate(0, 0)
	// Specify linear filter.
	op.Filter = ebiten.FilterLinear

	screen.DrawImage(offscreen, op)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("TPS: %0.2f, frame: %d, zoom: %.2f", ebiten.CurrentTPS(), zoomInt-startOffset, curZoom))
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
