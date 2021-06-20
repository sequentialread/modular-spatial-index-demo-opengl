package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"runtime"
	"strings"
	"time"

	spatialIndex "git.sequentialread.com/forest/modular-spatial-index"
	"github.com/go-gl/gl/v4.1-core/gl" // OR: github.com/go-gl/gl/v2.1/gl
	"github.com/go-gl/glfw/v3.2/glfw"
)

const dim = 512

const rainbowCount = float64(20)
const saturationFluctuationCount = float64(8)

var frames = 0

func main() {
	run_opengl_app(func() *image.RGBA {

		seconds := float64(time.Now().UnixNano()) / float64(int64(time.Second))

		rectX := int(float64(dim) * (float64(0.4) + math.Sin(seconds*float64(1.3))*float64(0.3)))
		rectY := int(float64(dim) * (float64(0.5) + math.Cos(seconds*float64(0.3))*float64(0.2)))
		rectSize := 9 + int(float64(40)*(float64(1)+math.Sin(seconds*float64(0.843))))
		rectMaxX := rectX + rectSize
		rectMaxY := rectY + rectSize

		inputMin, inputMax := spatialIndex.GetValidInputRange()
		_, outputMaxBytes := spatialIndex.GetOutputRange()
		curveLength := int(binary.BigEndian.Uint64(outputMaxBytes))
		//log.Printf("inputMin: %d, inputMax: %d, curveLength: %d", inputMin, inputMax, curveLength)

		remappedRectXMin := int(lerp(float64(inputMin), float64(inputMax), float64(rectX)/float64(dim)))
		remappedRectYMin := int(lerp(float64(inputMin), float64(inputMax), float64(rectY)/float64(dim)))
		remappedRectXMax := int(lerp(float64(inputMin), float64(inputMax), float64(rectX+rectSize)/float64(dim)))
		remappedRectSize := remappedRectXMax - remappedRectXMin

		byteRanges, err := spatialIndex.RectangleToIndexedRanges(remappedRectXMin, remappedRectYMin, remappedRectSize, remappedRectSize, 1)
		if err != nil {
			panic(err)
		}
		ranges := make([][]int, len(byteRanges))
		// log.Println("------------")
		for i, byteRange := range byteRanges {
			ranges[i] = []int{
				int(binary.BigEndian.Uint64(byteRange.Start)),
				int(binary.BigEndian.Uint64(byteRange.End)),
			}
			// log.Printf("Start: %x\n", byteRange.Start)
			// log.Printf("  End: %x\n", byteRange.End)
			// log.Printf("  Max: %x\n", outputMaxBytes)
		}
		// log.Println("------------")

		// outBytes, _ := json.MarshalIndent(ranges, "", "  ")
		// log.Println("outBytes: ", string(outBytes))

		rgba := image.NewRGBA(image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{dim, dim}})
		queriedArea := 0
		for x := 0; x < dim; x++ {
			for y := 0; y < dim; y++ {

				onVertical := (x == rectMaxX || x == rectX) && y >= rectY && y <= rectMaxY
				onHorizontal := (y == rectMaxY || y == rectY) && x >= rectX && x <= rectMaxX
				if onVertical || onHorizontal {
					rgba.Set(x, y, color.White)
					continue
				}

				remappedX := int(lerp(float64(inputMin), float64(inputMax), float64(x)/float64(dim)))
				remappedY := int(lerp(float64(inputMin), float64(inputMax), float64(y)/float64(dim)))
				if y > dim-20 {
					found := false

					xOnCurveNumberLine := int(lerp(float64(0), float64(curveLength), float64(x)/float64(dim)))
					for _, curveRange := range ranges {
						if xOnCurveNumberLine >= curveRange[0] && xOnCurveNumberLine <= curveRange[1] {
							found = true
						}
					}

					if found {
						rgba.Set(x, y, color.White)
					} else {
						rgba.Set(x, y, color.Black)
					}
					continue
				}

				curvePointBytes, err := spatialIndex.GetIndexedPoint(remappedX, remappedY)
				curvePoint := int(binary.BigEndian.Uint64(curvePointBytes))
				if err != nil {
					panic(err)
				}
				// if x*2 == y && !logged {
				// 	log.Printf("[%d,%d]: %d %d", x, y, curvePoint, int((float64(curvePoint)/float64(myCurve.N*myCurve.N))*1000))
				// }

				curveFloat := (float64(curvePoint) / float64(math.MaxInt64))
				//sat := (float64(2) + math.Sin(curveFloat*math.Pi*2*saturationFluctuationCount)) * float64(0.3333333)
				sat := 0.2
				// if curvePoint >= curvePoints[0] && curvePoint <= curvePoints[len(curvePoints)-1] {
				// 	sat = 1
				// }
				for _, rng := range ranges {
					if curvePoint >= rng[0] && curvePoint <= rng[1] {
						sat = 1
						queriedArea++
					}
				}
				hue := int(curveFloat*rainbowCount*float64(3600)) % 3600
				rainbow := hsvColor(float64(hue)*0.1, sat, sat)
				// uvColor := color.RGBA{
				// 	uint8((float32(x) / float32(width)) * float32(255)),
				// 	uint8((float32(y) / float32(width)) * float32(255)),
				// 	255,
				// 	255,
				// }

				rgba.Set(x, y, rainbow)
			}
		}

		if frames%10 == 0 {
			fmt.Printf("range count: %d, queriedArea: %d%%\n", len(ranges), int((float64(queriedArea)/float64(rectSize*rectSize))*float64(100)))
		}
		frames++

		return rgba
	})
}

func lerp(a, b, lerp float64) float64 {
	return a*(float64(1)-lerp) + b*lerp
}

func hsvColor(H, S, V float64) color.RGBA {
	Hp := H / 60.0
	C := V * S
	X := C * (1.0 - math.Abs(math.Mod(Hp, 2.0)-1.0))

	m := V - C
	r, g, b := 0.0, 0.0, 0.0

	switch {
	case 0.0 <= Hp && Hp < 1.0:
		r = C
		g = X
	case 1.0 <= Hp && Hp < 2.0:
		r = X
		g = C
	case 2.0 <= Hp && Hp < 3.0:
		g = C
		b = X
	case 3.0 <= Hp && Hp < 4.0:
		g = X
		b = C
	case 4.0 <= Hp && Hp < 5.0:
		r = X
		b = C
	case 5.0 <= Hp && Hp < 6.0:
		r = C
		b = X
	}

	return color.RGBA{uint8(int((m + r) * float64(255))), uint8(int((m + g) * float64(255))), uint8(int((m + b) * float64(255))), 0xff}
}

// -------------------------- OpenGL boilerplate -----------------------------------

const (
	vertexShaderSource = `
	    #version 410
	    in vec3 vp;
			in vec2 vertTexCoord;
			out vec2 fragTexCoord;
	    void main() {
					fragTexCoord = vertTexCoord;
	        gl_Position = vec4(vp, 1.0);
	    }
	` + "\x00"

	fragmentShaderSource = `
		#version 330
		uniform sampler2D tex;
		in vec2 fragTexCoord;
		out vec4 outputColor;
		void main() {
				outputColor = texture(tex, fragTexCoord);
		}
	` + "\x00"
)

var fullscreenQuad = []float32{
	//  X, Y, Z, U, V
	-1, 1, 0, 0, 1,
	-1, -1, 0, 0, 0,
	1, -1, 0, 1, 0,
	-1, 1, 0, 0, 1,
	1, 1, 0, 1, 1,
	1, -1, 0, 1, 0,
}

// basic OpenGL based display application copy and pasted from
// https://kylewbanks.com/blog/tutorial-opengl-with-golang-part-1-hello-opengl
func run_opengl_app(getImage func() *image.RGBA) {
	runtime.LockOSThread()

	window := initGlfw()
	defer glfw.Terminate()

	program := initOpenGL()

	for !window.ShouldClose() {
		texture, err := newTexture(getImage())
		if err != nil {
			panic(err)
		}

		draw(makeVertexArrayObject(fullscreenQuad, program), texture, window, program)
	}
}

func draw(vertexArrayObject uint32, texture uint32, window *glfw.Window, program uint32) {
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.UseProgram(program)

	gl.BindVertexArray(vertexArrayObject)

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, texture)

	gl.DrawArrays(gl.TRIANGLES, 0, int32(len(fullscreenQuad)/3))

	glfw.PollEvents()
	window.SwapBuffers()
}

func initOpenGL() uint32 {
	if err := gl.Init(); err != nil {
		panic(err)
	}
	version := gl.GoStr(gl.GetString(gl.VERSION))
	log.Println("OpenGL version", version)

	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		panic(err)
	}
	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		panic(err)
	}

	prog := gl.CreateProgram()
	gl.AttachShader(prog, vertexShader)
	gl.AttachShader(prog, fragmentShader)
	gl.LinkProgram(prog)
	return prog
}

// initGlfw initializes glfw and returns a Window to use.
func initGlfw() *glfw.Window {
	if err := glfw.Init(); err != nil {
		panic(err)
	}

	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 4) // OR 2
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(dim, dim, "hilbert test", nil, nil)
	if err != nil {
		panic(err)
	}
	window.MakeContextCurrent()

	return window
}

// texure coordinate stuff sourced from https://github.com/go-gl/example/blob/master/gl41core-cube/cube.go
func makeVertexArrayObject(points []float32, program uint32) uint32 {

	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(points)*4, gl.Ptr(points), gl.STATIC_DRAW)

	vertAttrib := uint32(gl.GetAttribLocation(program, gl.Str("vp\x00")))
	gl.EnableVertexAttribArray(vertAttrib)
	gl.VertexAttribPointerWithOffset(vertAttrib, 3, gl.FLOAT, false, 5*4, 0)

	texCoordAttrib := uint32(gl.GetAttribLocation(program, gl.Str("vertTexCoord\x00")))
	gl.EnableVertexAttribArray(texCoordAttrib)
	gl.VertexAttribPointerWithOffset(texCoordAttrib, 2, gl.FLOAT, false, 5*4, 3*4)

	return vao
}

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))

		return 0, fmt.Errorf("failed to compile %v: %v", source, log)
	}

	return shader, nil
}

func newTexture(rgba *image.RGBA) (uint32, error) {

	if rgba.Stride != rgba.Rect.Size().X*4 {
		return 0, fmt.Errorf("unsupported stride")
	}

	var texture uint32
	gl.GenTextures(1, &texture)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(
		gl.TEXTURE_2D,
		0,
		gl.RGBA,
		int32(rgba.Rect.Size().X),
		int32(rgba.Rect.Size().Y),
		0,
		gl.RGBA,
		gl.UNSIGNED_BYTE,
		gl.Ptr(rgba.Pix))

	return texture, nil
}
