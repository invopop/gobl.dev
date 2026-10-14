// editor.js -- ES module for GOBL Editor
// Initializes the CodeMirror editors: the GOBL source with JSON schema
// support, the input for documents to import, and a read-only viewer for
// export and import results. Once mounted, the editors are handed to the
// Alpine component through window._cmReady (created in <head>).

import { basicSetup, EditorView } from "codemirror";
import { EditorState, Compartment } from "@codemirror/state";
import { vsCodeLight } from "@fsegurai/codemirror-theme-vscode-light";
import { vsCodeDark } from "@fsegurai/codemirror-theme-vscode-dark";

function isDark() {
  return document.documentElement.classList.contains("dark");
}

const { jsonSchema, updateSchema } = await import("codemirror-json-schema");
const { xml } = await import("@codemirror/lang-xml");
const { json } = await import("@codemirror/lang-json");

const SCHEMA_PREFIX = "https://gobl.org/draft-0/";
let activeSchemaURL = null;
let debounceTimer;

async function loadSchemaFromDoc(view) {
  try {
    const text = view.state.doc.toString();
    const m = text.match(/"\$schema"\s*:\s*"([^"]+)"/);
    const url = m?.[1];
    if (!url || url === activeSchemaURL) return;
    if (!url.startsWith(SCHEMA_PREFIX)) return;

    activeSchemaURL = url;
    const path = url.slice(SCHEMA_PREFIX.length);
    const res = await fetch("/v0/schemas/" + path + "?bundle");
    if (res.ok) updateSchema(view, await res.json());
  } catch (e) {
    console.warn("Schema loading failed:", e);
  }
}

// Every editor gets its own theme compartment so all can follow dark mode.
const themes = [];

function createEditor(parent, extensions) {
  const theme = new Compartment();
  const view = new EditorView({
    state: EditorState.create({
      doc: "",
      extensions: [
        basicSetup,
        theme.of(isDark() ? vsCodeDark : vsCodeLight),
        EditorView.lineWrapping,
        ...extensions,
      ],
    }),
    parent,
  });
  themes.push({ view, theme });
  return view;
}

function setText(view, text) {
  view.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: text },
  });
}

const sourceParent = document.getElementById("source-gobl");
sourceParent.replaceChildren();

const source = createEditor(sourceParent, [
  jsonSchema(),
  EditorView.updateListener.of((update) => {
    if (update.docChanged) {
      clearTimeout(debounceTimer);
      debounceTimer = setTimeout(() => {
        loadSchemaFromDoc(update.view);
        window.dispatchEvent(new CustomEvent("gobl-source-change"));
      }, 300);
    }
  }),
]);

const input = createEditor(document.getElementById("source-xml"), [xml()]);

const viewerLanguage = new Compartment();
const viewer = createEditor(document.getElementById("viewer"), [
  EditorState.readOnly.of(true),
  viewerLanguage.of(xml()),
]);

window._cmReadyResolve({
  source,
  input,
  setSource(text) {
    setText(source, text);
    loadSchemaFromDoc(source);
  },
  setInput(text) {
    setText(input, text);
  },
  // setViewer shows the text in the read-only viewer, highlighted as "xml"
  // or "json".
  setViewer(text, lang) {
    viewer.dispatch({
      changes: { from: 0, to: viewer.state.doc.length, insert: text },
      effects: viewerLanguage.reconfigure(lang === "json" ? json() : xml()),
    });
  },
  setDark(dark) {
    const theme = dark ? vsCodeDark : vsCodeLight;
    for (const t of themes) {
      t.view.dispatch({ effects: t.theme.reconfigure(theme) });
    }
  },
});
