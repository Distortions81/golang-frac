package main

import (
	"fmt"
	"image/color"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/remeh/sizedwaitgroup"
)

const (
	autoZoom    = true
	startOffset = 48
	superSample = 32
	winWidth    = 1024
	winHeight   = 1024
	maxIters    = 1024 * 10
	offX        = -0.77568377
	offY        = 0.13646737
	zoomSpeed   = 10
	wheelSpeed  = 0.05
	gamma       = 0.45454545454545 // 2.2
	dither      = 2
)

var (
	renderWidth  int = winWidth * superSample
	renderHeight int = winHeight * superSample
)

var curZoom float64 = 1.0
var zoomInt int = startOffset

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
	ebitenutil.DebugPrint(screen, fmt.Sprintf("UPS: %0.2f, as: %d, z: %0.2f, w: %0.2f, t: %d", ebiten.CurrentTPS(), zoomInt-startOffset, curZoom, g.y, numThreads))
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
	offscreenPix []uint16
	palette      [maxIters]uint16
	numThreads   = runtime.NumCPU()
	count        uint64
)

func lut(it uint16) (c uint16) {
	if it >= maxIters {
		return 0xffff
	}

	//Dither
	/*f := rand.Float64()
	if f > 0.5 {
		return palette[it]
	} else {
		return palette[it] + dither

	} */

	return palette[it]
}

func updateOffscreen(centerX, centerY, size float64) {
	count++

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
				var it uint16
				for ; it < maxIters; it++ {
					z = z*z + c
					if real(z)*real(z)+imag(z)*imag(z) > 4 {
						break
					}
				}
				p := (i + j*renderWidth)
				offscreenPix[p] = it
			}

		}(j)
	}
	swg.Wait()

	//Convert to byte
	var nb = ebiten.NewImage(renderWidth, renderHeight)
	for xx := 0; xx < renderWidth; xx++ {
		for yy := 0; yy < renderHeight; yy++ {
			a := lut(offscreenPix[xx+yy*renderWidth])
			nb.Set(xx, yy, color.RGBA64{a, a, a, 0xffff})
		}
	}

	offscreen.DrawImage(nb, &ebiten.DrawImageOptions{})

	buf := fmt.Sprintf("out/%v.png", count)
	f, err := os.Create(buf)
	if err != nil {
		log.Fatal(err)
	} else {
		err = png.Encode(f, nb)
		if err != nil {
			log.Fatal(err)
		}
		f.Close()
	}

}

func init() {

	rand.Seed(time.Now().UnixNano())
	buf := fmt.Sprintf("Threads found: %x", numThreads)
	fmt.Println(buf)
	if numThreads < 4 {
		numThreads = 2
	}

	fmt.Println("Allocating image...")

	/* Max image size */
	if renderWidth > 32768 {
		renderWidth = 32768
		fmt.Println("renderWidth > 32768, truncating...")
	}
	if renderHeight > 32768 {
		renderHeight = 32768
		fmt.Println("renderHeight > 32768, truncating...")
	}

	offscreen = ebiten.NewImage(renderWidth, renderHeight)
	offscreenPix = make([]uint16, renderWidth*renderHeight)

	fmt.Printf("complete!\n")

	fmt.Printf("Building gamma table...")
	swg := sizedwaitgroup.New((numThreads))
	for i := range palette {
		swg.Add()
		go func(i int) {
			defer swg.Done()

			palette[i] = uint16(math.Pow(float64(i)/float64(maxIters+1), gamma) * float64(0xffff))
			//buf := fmt.Sprintf("%d, ", palette[i])
			//fmt.Print(buf)
		}(i)
	}

	swg.Wait()
	fmt.Printf("complete!\n")
}
