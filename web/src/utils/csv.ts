export const parseCsv = (text: string): string[][] => {
  return text
    .trim()
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => line.split(',').map((v) => v.trim()));
};

export const toCsv = (rows: string[][]): string => rows.map((r) => r.join(',')).join('\n');

export const downloadCsv = (filename: string, content: string) => {
  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  const link = document.createElement('a');
  link.href = URL.createObjectURL(blob);
  link.download = filename;
  link.click();
  URL.revokeObjectURL(link.href);
};
