import { downloadCsv } from './csv';

export const exportBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
};

export const exportCsvRows = (rows: string[][], filename: string) =>
  downloadCsv(filename, rows.map((r) => r.join(',')).join('\n'));
