// editor-data.js -- Alpine.js data components for the GOBL editor.
// Loaded as a synchronous script (before Alpine.js which is deferred)
// so the alpine:init listener is registered in time.
const MODES = ["build", "convert"];

document.addEventListener("alpine:init", () => {
  const boot = readBootstrap();

  Alpine.data("editor", () => ({
    // "build" replaces the GOBL source with the built document. "convert"
    // shows the GOBL source beside a document in another format: exporting
    // writes the GOBL source into that format, and importing reads the other
    // document back into GOBL.
    mode: initialMode(),
    loading: false,
    envelop: false,
    error: null,
    // The pane the last successful action updated, with a counter so each
    // one creates a fresh FlashMessage.
    done: { pane: "", n: 0 },

    examples: boot.examples || [],
    exampleID: "",
    sampleID: "",

    // Labels of the export formats registered with GOBL's convert package,
    // by key, and those that accept the GOBL source, as checked by the
    // server: only a format's converter can tell whether it accepts a
    // document.
    labels: boot.formats || {},
    available: [],
    checking: false,
    checks: 0,
    format: stored("editor-format") || "",
    // Name of the format of the other document, once known.
    inputTitle: "",

    cm: null,

    async init() {
      window.addEventListener("keydown", (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
          e.preventDefault();
          this.run(this.defaultAction());
        }
      });
      window.addEventListener("gobl-source-change", () => {
        if (this.mode === "convert") this.checkFormats();
      });
      this.$watch("format", (v) => v && store("editor-format", v));

      this.cm = await window._cmReady;
      await this.loadExample(this.findExample(stored("editor-example"))?.id || boot.defaultExample);
      if (this.mode === "convert") await this.enterConvert();
    },

    findExample(id) {
      return this.examples.find((e) => e.id === id);
    },

    // defaultAction is run with ⌘/Ctrl+Enter: building, or in convert mode,
    // the action that reads from the focused editor.
    defaultAction() {
      if (this.mode === "build") return "build";
      return this.cm?.input.hasFocus ? "import" : "export";
    },

    setMode(mode) {
      if (mode === this.mode) return;
      this.mode = mode;
      store("editor-mode", mode);
      this.error = null;
      if (mode === "convert") this.enterConvert();
    },

    // enterConvert checks the formats for the GOBL source, and fills the
    // other editor with a sample when it is empty.
    async enterConvert() {
      await this.checkFormats();
      if (this.cm.input.state.doc.length) return;
      const current = this.findExample(this.exampleID);
      const ex =
        this.findExample(stored("editor-sample")) ||
        (current?.formats?.length ? current : null) ||
        this.examples.find((e) => e.formats?.length);
      if (ex) await this.loadSample(ex.id);
    },

    // checkFormats asks the server which formats accept the GOBL source,
    // keeping the selected format among them. Documents that do not build
    // have none, and the reason is shown as an error.
    async checkFormats() {
      const seq = ++this.checks;
      this.checking = true;
      try {
        const res = await fetch("/v0/formats", {
          method: "POST",
          headers: { "Content-Type": "text/plain" },
          body: this.cm.source.state.doc.toString(),
        });
        const body = await res.json().catch(() => ({ message: "format check failed: " + res.status }));
        // A newer check started while this one was running.
        if (seq !== this.checks) return;
        if (!res.ok) {
          this.available = [];
          this.error = body;
        } else {
          this.available = body.formats
            .filter((f) => f.accepted)
            .map((f) => ({ key: f.key, label: this.labels[f.key] || f.key }));
          if (this.error && !this.loading) this.error = null;
        }
        const keys = this.available.map((f) => f.key);
        if (!keys.includes(this.format)) this.format = keys[0] || "";
      } finally {
        if (seq === this.checks) this.checking = false;
      }
    },

    // loadExample replaces the GOBL source with the example, selecting its
    // most specific format, listed first.
    async loadExample(id) {
      const ex = this.findExample(id);
      if (!ex || !this.cm) return;
      this.exampleID = id;
      store("editor-example", id);
      this.error = null;
      if (ex.formats?.length) this.format = ex.formats[0];
      this.cm.setSource(JSON.stringify(ex.doc, null, 2));
      if (this.mode === "convert") await this.checkFormats();
    },

    // loadSample replaces the other document with the example exported into
    // its most specific format.
    async loadSample(id) {
      const ex = this.findExample(id);
      const key = ex?.formats?.[0];
      if (!key || !this.cm) return;
      this.sampleID = id;
      store("editor-sample", id);
      this.error = null;
      const out = await this.post("/v0/export?format=" + encodeURIComponent(key), ex.doc);
      if (!out) return;
      this.cm.setInput(out.text);
      this.inputTitle = this.labels[out.format] || out.format;
    },

    async run(action) {
      if (!this.cm || this.loading) return;
      this.loading = true;
      this.error = null;
      try {
        await this[action]();
      } catch (e) {
        this.error = { message: e.message };
      } finally {
        this.loading = false;
      }
    },

    flash(pane) {
      this.done = { pane, n: this.done.n + 1 };
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
      this.flash("source");
    },

    // export writes the GOBL source, built by the server and left untouched
    // here, into the selected format in the other editor.
    async export() {
      if (!this.format) return;
      const out = await this.post(
        "/v0/export?format=" + encodeURIComponent(this.format),
        this.cm.source.state.doc.toString(),
      );
      if (!out) return;
      this.cm.setInput(out.text);
      // The other editor no longer holds a sample.
      this.sampleID = "";
      this.inputTitle = this.labels[out.format] || out.format;
      this.flash("input");
    },

    // import reads the other document, detecting its format, into the GOBL
    // source.
    async import() {
      const out = await this.post("/v0/import", this.cm.input.state.doc.toString());
      if (!out) return;
      this.inputTitle = this.labels[out.format] || out.format || "";
      // Export back into the detected format by default.
      if (out.format) this.format = out.format;
      this.cm.setSource(JSON.stringify(JSON.parse(out.text), null, 2));
      // The GOBL source no longer holds an example.
      this.exampleID = "";
      this.flash("source");
      await this.checkFormats();
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

// initialMode restores the last mode, mapping the former export and import
// modes onto convert.
function initialMode() {
  const m = stored("editor-mode");
  if (m === "export" || m === "import") return "convert";
  return MODES.includes(m) ? m : "build";
}

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
