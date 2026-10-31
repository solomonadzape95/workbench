(() => {
  const canvas = document.getElementById('dither');
  if (!canvas) return;
  const ctx = canvas.getContext('2d', { alpha: true });
  if (!ctx) return;

  const bayer = [
    0, 32, 8, 40, 2, 34, 10, 42,
    48, 16, 56, 24, 50, 18, 58, 26,
    12, 44, 4, 36, 14, 46, 6, 38,
    60, 28, 52, 20, 62, 30, 54, 22,
    3, 35, 11, 43, 1, 33, 9, 41,
    51, 19, 59, 27, 49, 17, 57, 25,
    15, 47, 7, 39, 13, 45, 5, 37,
    63, 31, 55, 23, 61, 29, 53, 21,
  ];
  const amber = [245, 167, 66];
  const dim = [156, 107, 37];
  const scale = 4;
  let w = 0;
  let h = 0;
  let frame = 0;
  let raf = 0;
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  function resize() {
    w = Math.max(1, Math.ceil(window.innerWidth / scale));
    h = Math.max(1, Math.ceil(window.innerHeight / scale));
    canvas.width = w;
    canvas.height = h;
    ctx.imageSmoothingEnabled = false;
  }

  function draw(now) {
    const img = ctx.createImageData(w, h);
    const data = img.data;
    const t = reduce ? 0 : now * 0.00012;
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const nx = x / w - 0.5;
        const ny = y / h - 0.5;
        const glow = Math.exp(-((nx * nx * 2.6) + (ny * ny * 3.8)) * 4.2);
        const wave = 0.5 + 0.5 * Math.sin(x * 0.065 + t * 2.1 + Math.cos(y * 0.09 - t));
        const value = glow * 0.62 + wave * 0.055;
        const threshold = (bayer[(y & 7) * 8 + (x & 7)] + 0.5) / 64;
        const index = (y * w + x) * 4;
        if (value > threshold * 0.9) {
          const mix = Math.min(1, Math.max(0, value * 1.8));
          data[index] = Math.round(dim[0] + (amber[0] - dim[0]) * mix);
          data[index + 1] = Math.round(dim[1] + (amber[1] - dim[1]) * mix);
          data[index + 2] = Math.round(dim[2] + (amber[2] - dim[2]) * mix);
          data[index + 3] = 255;
        }
      }
    }
    ctx.putImageData(img, 0, 0);
    frame++;
    if (!reduce && frame % 2 === 0) {
      raf = requestAnimationFrame(draw);
    }
  }

  function start() {
    resize();
    cancelAnimationFrame(raf);
    if (reduce) {
      draw(0);
    } else {
      raf = requestAnimationFrame(draw);
    }
  }

  window.addEventListener('resize', start, { passive: true });
  start();
})();
