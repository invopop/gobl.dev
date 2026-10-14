// editor-data.js -- Alpine.js data components for the GOBL editor.
// Loaded as a synchronous script (before Alpine.js which is deferred)
// so the alpine:init listener is registered in time.
const ENVELOPE_SCHEMA = "https://gobl.org/draft-0/envelope";
const INVOICE_SCHEMA = "https://gobl.org/draft-0/bill/invoice";
const MODES = ["build", "export", "import"];

document.addEventListener("alpine:init", () => {
  const boot = readBootstrap();

  Alpine.data("editor", () => ({
    // "build" replaces the GOBL source with the built document, "export"
    // builds it internally and shows it converted into a format, and
    // "import" converts the input into GOBL.
    mode: MODES.includes(stored("editor-mode")) ? stored("editor-mode") : "build",
    loading: false,
    envelop: false,
    error: null,
    // Counter so each successful run creates a fresh FlashMessage.
    success: 0,

    examples: boot.examples || [],
    exampleID: "",
    invoiceSchema: INVOICE_SCHEMA,

    // Export formats registered with GOBL's convert package, and the addons
    // each addon requires, used to work out which formats apply.
    formats: boot.formats || [],
    requires: boot.requires || {},
    format: stored("editor-format") || "",
    viewerTitle: "",

    // Schema and addons of the GOBL source, updated as it is edited.
    docSchema: "",
    docAddons: [],

    cm: null,

    async init() {
      window.addEventListener("keydown", (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
          e.preventDefault();
          this.run();
        }
      });
      window.addEventListener("gobl-source-change", () => this.inspectSource());
      this.$watch("format", (v) => v && store("editor-format", v));

      this.cm = await window._cmReady;
      const id = this.examples.some((e) => e.id === stored("editor-example"))
        ? stored("editor-example")
        : boot.defaultExample;
      await this.loadExample(id);
    },

    get actionLabel() {
      return { build: "Build", export: "Export", import: "Import" }[this.mode];
    },

    // available lists the formats the GOBL source can be exported into.
    get available() {
      return this.formatsFor(this.docAddons, this.docSchema);
    },

    // formatsFor lists the export formats for documents of the schema that
    // declare the addons. Addons are expanded with those they require, as
    // GOBL does when building, and every addon a format lists must be
    // present.
    formatsFor(addons, schema) {
      const all = this.expandAddons(addons || []);
      return this.formats.filter(
        (f) =>
          f.export.includes(schema) &&
          (f.addons || []).every((a) => all.includes(a)),
      );
    },

    expandAddons(addons) {
      const out = [...addons];
      for (let i = 0; i < out.length; i++) {
        for (const r of this.requires[out[i]] || []) {
          if (!out.includes(r)) out.push(r);
        }
      }
      return out;
    },

    // inspectSource reads the schema and addons of the GOBL source, and
    // keeps the selected format among those available.
    inspectSource() {
      try {
        let doc = JSON.parse(this.cm.source.state.doc.toString());
        if (doc?.$schema === ENVELOPE_SCHEMA) doc = doc.doc;
        this.docSchema = doc?.$schema || "";
        this.docAddons = Array.isArray(doc?.$addons) ? doc.$addons : [];
      } catch {
        // Keep the previous values while the source is being edited.
        return;
      }
      const keys = this.available.map((f) => f.key);
      if (!keys.includes(this.format)) this.format = keys[0] || "";
    },

    setMode(mode) {
      if (mode === this.mode) return;
      this.mode = mode;
      store("editor-mode", mode);
      this.error = null;
      this.cm?.setViewer("");
      this.viewerTitle = "";
      if (mode === "export") {
        this.run();
      } else if (mode === "import") {
        if (this.cm?.input.state.doc.length) {
          this.run();
        } else {
          this.loadExample(this.exampleID);
        }
      }
    },

    // loadExample replaces the GOBL source with the example, or in import
    // mode, the input with the example exported into its first format.
    async loadExample(id) {
      const ex = this.examples.find((e) => e.id === id);
      if (!ex || !this.cm) return;
      this.exampleID = id;
      store("editor-example", id);
      this.error = null;

      if (this.mode === "import") {
        const f = this.formatsFor(ex.addons, INVOICE_SCHEMA)[0];
        if (!f) {
          this.viewerTitle = "Paste a document to import, or choose an example with an export format";
          return;
        }
        const out = await this.post("/v0/export?format=" + encodeURIComponent(f.key), ex.doc);
        if (!out) return;
        this.cm.setInput(out.text);
        await this.run();
        return;
      }

      this.cm.setSource(JSON.stringify(ex.doc, null, 2));
      this.inspectSource();
      if (this.mode === "export") await this.run();
    },

    async run() {
      if (!this.cm || this.loading) return;
      this.loading = true;
      this.error = null;
      this.success = 0;
      try {
        await this[this.mode]();
      } catch (e) {
        this.error = { message: e.message };
      } finally {
        this.loading = false;
      }
    },

    async build() {
      const ed = this.cm.source;
      const payload = JSON.parse(ed.state.doc.toString());
      const out = await this.post("/v0/build", { data: payload, envelop: this.envelop });
      if (!out) return;

      const pretty = JSON.stringify(JSON.parse(out.text), null, 2);
      const scrollTop = ed.scrollDOM.scrollTop;
      const { anchor, head } = ed.state.selection.main;
      ed.dispatch({
        changes: { from: 0, to: ed.state.doc.length, insert: pretty },
        selection: {
          anchor: Math.min(anchor, pretty.length),
          head: Math.min(head, pretty.length),
        },
      });
      requestAnimationFrame(() => {
        ed.scrollDOM.scrollTop = scrollTop;
      });
      ed.focus();
      this.success++;
    },

    // export leaves the GOBL source untouched: the server builds it and
    // converts the result.
    async export() {
      if (!this.format) return;
      const out = await this.post(
        "/v0/export?format=" + encodeURIComponent(this.format),
        this.cm.source.state.doc.toString(),
      );
      if (!out) {
        this.viewerTitle = "";
        this.cm.setViewer("");
        return;
      }
      const f = this.formats.find((x) => x.key === out.format);
      this.viewerTitle = f ? f.label : out.format;
      this.cm.setViewer(out.text, "xml");
      this.success++;
    },

    async import() {
      const out = await this.post("/v0/import", this.cm.input.state.doc.toString());
      if (!out) {
        this.viewerTitle = "";
        this.cm.setViewer("");
        return;
      }
      this.viewerTitle = "GOBL envelope";
      this.cm.setViewer(JSON.stringify(JSON.parse(out.text), null, 2), "json");
      this.success++;
    },

    // post sends the body (JSON encoded unless already a string) and returns
    // the response text and the format used, or null after showing the
    // error.
    async post(url, body) {
      const raw = typeof body === "string";
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": raw ? "text/plain" : "application/json" },
        body: raw ? body : JSON.stringify(body),
      });
      if (!res.ok) {
        this.error = await res.json().catch(() => ({
          message: "request failed: " + res.status,
        }));
        return null;
      }
      return { text: await res.text(), format: res.headers.get("GOBL-Format") };
    },
  }));

  Alpine.data("darkModeToggle", () => ({
    dark: false,
    init() {
      const s = stored("dark-mode");
      this.dark =
        s !== null ? s === "true" : document.documentElement.classList.contains("dark");
      document.documentElement.classList.toggle("dark", this.dark);
      this.applyEditorTheme();
    },
    toggle() {
      this.dark = !this.dark;
      document.documentElement.classList.toggle("dark", this.dark);
      store("dark-mode", this.dark);
      this.applyEditorTheme();
    },
    async applyEditorTheme() {
      const cm = await window._cmReady;
      cm.setDark(this.dark);
    },
  }));
});

function readBootstrap() {
  const el = document.getElementById("editor-bootstrap");
  try {
    return el ? JSON.parse(el.textContent) : {};
  } catch (e) {
    console.warn("Failed to parse editor bootstrap:", e);
    return {};
  }
}

// stored and store wrap localStorage, which may be unavailable.
function stored(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function store(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Preferences are not kept.
  }
}
