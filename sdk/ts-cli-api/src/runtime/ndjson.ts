// Purpose: split a byte stream into NDJSON lines.
// Inputs: a stream reader. Outputs: non-empty lines as strings.
// Constraints: UTF-8 safe across chunk boundaries; cancels the reader on early exit.

interface Reader {
  read(): Promise<{ done: boolean; value?: Uint8Array }>;
  cancel(): Promise<void>;
}

export async function* readLines(reader: Reader): AsyncGenerator<string, void, undefined> {
  const decoder = new TextDecoder();
  let buffer = "";
  let finished = false;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let nl = buffer.indexOf("\n");
      while (nl >= 0) {
        const line = buffer.slice(0, nl).trim();
        buffer = buffer.slice(nl + 1);
        if (line) yield line;
        nl = buffer.indexOf("\n");
      }
    }
    buffer += decoder.decode();
    if (buffer.trim()) yield buffer.trim();
    finished = true;
  } finally {
    if (!finished) await reader.cancel().catch(() => undefined);
  }
}
