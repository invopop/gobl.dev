// editor.js -- ES module for GOBL Editor
// Initializes the main editing CodeMirror and a read-only XML viewer.
// window._cmReady + window._cmReadyResolve are created by an inline script
// in <head>; this module resolves the promise once CodeMirror is mounted.

import { basicSetup, EditorView } from "codemirror";
import { EditorState, Compartment } from "@codemirror/state";
import { vsCodeLight } from "@fsegurai/codemirror-theme-vscode-light";
import { vsCodeDark } from "@fsegurai/codemirror-theme-vscode-dark";

// Compartments allow dynamic reconfiguration.
const editorThemeCompartment = new Compartment();
const viewerThemeCompartment = new Compartment();

function isDark() {
  return document.documentElement.classList.contains("dark");
}

const { jsonSchema, updateSchema } = await import("codemirror-json-schema");
const { xml: xmlLang } = await import("@codemirror/lang-xml");

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

// Mount the main editor inside #editor-pane.
const editorMount = document.getElementById("editor-pane");
editorMount.replaceChildren();

const editor = new EditorView({
  state: EditorState.create({
    doc: "",
    extensions: [
      basicSetup,
      editorThemeCompartment.of(isDark() ? vsCodeDark : vsCodeLight),
      EditorView.lineWrapping,
      jsonSchema(),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          clearTimeout(debounceTimer);
          debounceTimer = setTimeout(
            () => loadSchemaFromDoc(update.view),
            500,
          );
        }
      }),
    ],
  }),
  parent: editorMount,
});

window._cmEditor = editor;

// Lazily-created read-only viewer for XML output.
let viewerView = null;

function ensureViewer() {
  if (viewerView) return viewerView;
  const parent = document.getElementById("viewer-xml");
  if (!parent) return null;
  viewerView = new EditorView({
    state: EditorState.create({
      doc: "",
      extensions: [
        basicSetup,
        viewerThemeCompartment.of(isDark() ? vsCodeDark : vsCodeLight),
        EditorView.lineWrapping,
        EditorView.editable.of(false),
        EditorState.readOnly.of(true),
        xmlLang(),
      ],
    }),
    parent,
  });
  return viewerView;
}

window._cmSetViewerXML = (text) => {
  const v = ensureViewer();
  if (!v) return;
  v.dispatch({
    changes: { from: 0, to: v.state.doc.length, insert: text },
  });
};

window._cmSetDark = (dark) => {
  const theme = dark ? vsCodeDark : vsCodeLight;
  editor.dispatch({
    effects: editorThemeCompartment.reconfigure(theme),
  });
  if (viewerView) {
    viewerView.dispatch({
      effects: viewerThemeCompartment.reconfigure(theme),
    });
  }
};

window._cmSetEditorDoc = (text) => {
  editor.dispatch({
    changes: { from: 0, to: editor.state.doc.length, insert: text },
  });
  loadSchemaFromDoc(editor);
};

if (window._cmReadyResolve) window._cmReadyResolve();
