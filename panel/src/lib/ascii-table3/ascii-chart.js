// colors.js
export const black = "\x1b[30m";
export const red = "\x1b[31m";
export const green = "\x1b[32m";
export const yellow = "\x1b[33m";
export const blue = "\x1b[34m";
export const magenta = "\x1b[35m";
export const cyan = "\x1b[36m";
export const lightgray = "\x1b[37m";
export const defaultColor = "\x1b[39m";
export const darkgray = "\x1b[90m";
export const lightred = "\x1b[91m";
export const lightgreen = "\x1b[92m";
export const lightyellow = "\x1b[93m";
export const lightblue = "\x1b[94m";
export const lightmagenta = "\x1b[95m";
export const lightcyan = "\x1b[96m";
export const white = "\x1b[97m";
export const reset = "\x1b[0m";

export function colored(char, color) {
    return color ? color + char + reset : char;
}

export function plot(series, cfg = {}) {
    if (typeof(series[0]) === "number") {
        series = [series];
    }

    let min = cfg.min ?? series[0][0];
    let max = cfg.max ?? series[0][0];

    series.forEach(s => {
        s.forEach(value => {
            min = Math.min(min, value);
            max = Math.max(max, value);
        });
    });

    let defaultSymbols = ['┼', '┤', '╶', '╴', '─', '╰', '╭', '╮', '╯', '│'];
    let range = max - min;
    let offset = cfg.offset ?? 3;
    let padding = cfg.padding ?? '           ';
    let height = cfg.height ?? range;
    let colors = cfg.colors ?? [];
    let ratio = range !== 0 ? height / range : 1;
    let min2 = Math.round(min * ratio);
    let max2 = Math.round(max * ratio);
    let rows = Math.abs(max2 - min2);
    let width = series.reduce((acc, val) => Math.max(acc, val.length), 0) + offset;
    let symbols = cfg.symbols ?? defaultSymbols;
    let format = cfg.format ?? (x => (padding + x.toFixed(2)).slice(-padding.length));

    let result = new Array(rows + 1).fill(null).map(() => new Array(width).fill(' '));

    for (let y = min2; y <= max2; ++y) {
        let label = format(max - (y - min2) * range / rows);
        result[y - min2][Math.max(offset - label.length, 0)] = label;
        result[y - min2][offset - 1] = y === 0 ? symbols[0] : symbols[1];
    }

    series.forEach((s, j) => {
        let currentColor = colors[j % colors.length];
        let y0 = Math.round(s[0] * ratio) - min2;
        result[rows - y0][offset - 1] = colored(symbols[0], currentColor);

        for (let x = 0; x < s.length - 1; x++) {
            let y0 = Math.round(s[x] * ratio) - min2;
            let y1 = Math.round(s[x + 1] * ratio) - min2;
            let symbolIndex = y0 === y1 ? 4 : y0 > y1 ? 5 : 6;
            result[rows - y1][x + offset] = colored(symbols[symbolIndex], currentColor);
            result[rows - y0][x + offset] = colored(symbols[y0 > y1 ? 7 : 8], currentColor);

            for (let y = Math.min(y0, y1) + 1; y < Math.max(y0, y1); y++) {
                result[rows - y][x + offset] = colored(symbols[9], currentColor);
            }
        }
    });

    return result.map(row => row.join('')).join('\n');
}
