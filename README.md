# modular-spatial-index-demo-opengl

![animated gif showing query zone and selected curve area](https://sequentialread.com/content/images/2021/06/hilbert.gif)

This demo was based on [Golang OpenGL tutorial by kylewbanks.com](https://kylewbanks.com/blog/tutorial-opengl-with-golang-part-1-hello-opengl).

### [modular-spatial-index](https://git.sequentialread.com/forest/modular-spatial-index)

[modular-spatial-index](https://git.sequentialread.com/forest/modular-spatial-index) is a simple spatial index adapter for key/value databases (or RDBMS if you want), based on https://github.com/google/hilbert.

Read amplification for range queries is aproximately like 2x-3x in terms of IOPS and bandwidth compared to a 1-dimensional query.

But that constant factor on top of your fast key/value database is a low price to pay for a whole new dimension, right? It's certainly better than the naive approach.

See https://sequentialread.com/building-a-spatial-index-supporting-range-query-using-space-filling-hilbert-curve
for more information.

MIT license 

