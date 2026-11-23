package main

// faviconSVG 是门户的站点图标（G 版）：蓝色渐变圆角方块上，一个细笔画的白色 C，
// 缺口右侧三块错落的小方块——C 代表 ChainWeaver，方块代表汇入门户的各个站点。
// 线宽 5.6 与方块 6.8 是为 16px 浏览器标签专门调的：再细会在标签尺寸下糊成一团。
// 渐变用 linearGradient 而非纯色，和门户按钮、签的选中态保持同一套蓝色。
const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">` +
	`<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">` +
	`<stop offset="0" stop-color="#2b7fff"/><stop offset="1" stop-color="#0040b0"/>` +
	`</linearGradient></defs>` +
	`<rect width="64" height="64" rx="14" fill="url(#g)"/>` +
	`<path d="M42.5 16.5A17.8 17.8 0 1 0 42.5 47.5" fill="none" stroke="#fff" ` +
	`stroke-width="5.6" stroke-linecap="round"/>` +
	`<rect x="26" y="21.5" width="6.8" height="6.8" rx="2" fill="#fff" opacity=".95"/>` +
	`<rect x="32.4" y="28.6" width="6.8" height="6.8" rx="2" fill="#fff" opacity=".78"/>` +
	`<rect x="26" y="35.7" width="6.8" height="6.8" rx="2" fill="#fff" opacity=".6"/>` +
	`</svg>`
