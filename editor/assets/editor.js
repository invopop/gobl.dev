// editor.js -- ES module for GOBL Editor
// Initializes CodeMirror with JSON schema support.

import { basicSetup, EditorView } from "codemirror";
import { EditorState, Compartment } from "@codemirror/state";
import { materialLight } from "@fsegurai/codemirror-theme-material-light";
import { materialDark } from "@fsegurai/codemirror-theme-material-dark";

// Theme compartment allows dynamic reconfiguration.
const themeCompartment = new Compartment();

function isDark() {
  return document.documentElement.classList.contains("dark");
}

const defaultDoc = JSON.stringify(
  {
    $schema: "https://gobl.org/draft-0/bill/invoice",
    currency: "USD",
    issue_date: new Date().toISOString().slice(0, 10),
    supplier: {
      name: "Acme Inc.",
      tax_id: {
        country: "US",
      },
    },
    customer: {
      name: "Sample Customer",
    },
    lines: [
      {
        quantity: "10",
        item: {
          name: "Development Services",
          price: "100.00",
        },
        taxes: [
          {
            cat: "ST",
            percent: "8.25%",
          },
        ],
      },
    ],
  },
  null,
  2,
);

const { jsonSchema, updateSchema } = await import("codemirror-json-schema");

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

const container = document.getElementById("editor-container");
container.replaceChildren();

const editor = new EditorView({
  state: EditorState.create({
    doc: defaultDoc,
    extensions: [
      basicSetup,
      themeCompartment.of(isDark() ? materialDark : materialLight),
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
  parent: container,
});

window._cmEditor = editor;
window._cmSetDark = (dark) => {
  editor.dispatch({
    effects: themeCompartment.reconfigure(dark ? materialDark : materialLight),
  });
};
loadSchemaFromDoc(editor);

