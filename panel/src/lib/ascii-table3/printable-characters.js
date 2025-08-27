// Assuming this is saved as printable-characters.js
const ansiEscapeCode = '[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-PRZcf-nqry=><]';
const zeroWidthCharacterExceptNewline = '\u0000-\u0008\u000B-\u0019\u001b\u009b\u00ad\u200b\u2028\u2029\ufeff\ufe00-\ufe0f';
const zeroWidthCharacter = '\n' + zeroWidthCharacterExceptNewline;
const zeroWidthCharactersExceptNewline = new RegExp('(?:' + ansiEscapeCode + ')|[' + zeroWidthCharacterExceptNewline + ']', 'g');
const zeroWidthCharacters = new RegExp('(?:' + ansiEscapeCode + ')|[' + zeroWidthCharacter + ']', 'g');
const partition = new RegExp('((?:' + ansiEscapeCode + ')|[\t' + zeroWidthCharacter + '])?([^\t' + zeroWidthCharacter + ']*)', 'g');

export const strlen = s => Array.from(s.replace(zeroWidthCharacters, '')).length;

export const isBlank = s => s.replace(zeroWidthCharacters, '')
    .replace(/\s/g, '')
    .length === 0;

export const blank = s => Array.from(s.replace(zeroWidthCharactersExceptNewline, ''))
    .map(x => ((x === '\t') || (x === '\n')) ? x : ' ')
    .join('');

export function partitionString(s) {
    let spans = [];
    let m;
    while ((partition.lastIndex !== s.length) && (m = partition.exec(s))) {
        spans.push([m[1] || '', m[2]]);
    }
    partition.lastIndex = 0; // reset
    return spans;
}

export function first(s, n) {
    let result = '', length = 0;
    for (const [nonPrintable, printable] of partitionString(s)) {
        const text = Array.from(printable).slice(0, n - length);
        result += nonPrintable + text.join('');
        length += text.length;
    }
    return result;
}
