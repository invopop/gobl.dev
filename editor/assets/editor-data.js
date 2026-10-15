// editor-data.js -- Alpine.js data components for the GOBL editor.
// Loaded as a synchronous script (before Alpine.js which is deferred)
// so the alpine:init listener is registered in time.
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

    // Labels of the export formats registered with GOBL's convert package,
    // by key, and those that accept the GOBL source, as checked by the
    // server: only a format's converter can tell whether it accepts a
    // document.
    labels: boot.formats || {},
    available: [],
    checking: false,
    checks: 0,
    format: stored("editor-format") || "",
    viewerTitle: "",

    cm: null,

    async init() {
      window.addEventListener("keydown", (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
          e.preventDefault();
          this.run();
        }
      });
      // In export mode, edits are checked and exported as they are made.
      window.addEventListener("gobl-source-change", async () => {
        if (this.mode !== "export") return;
        await this.checkFormats();
        if (this.format) await this.run();
        else this.clearViewer();
      });
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

    setMode(mode) {
      if (mode === this.mode) return;
      this.mode = mode;
      store("editor-mode", mode);
      this.error = null;
      if (this.cm) this.clearViewer();
      if (mode === "export") {
        this.checkFormats().then(() => this.run());
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
        const key = ex.formats?.[0];
        if (!key) {
          this.viewerTitle = "Paste a document to import, or choose an example with an export format";
          return;
        }
        const out = await this.post("/v0/export?format=" + encodeURIComponent(key), ex.doc);
        if (!out) return;
        this.cm.setInput(out.text);
        await this.run();
        return;
      }

      // Prefer the example's most specific format, listed first.
      if (ex.formats?.length) this.format = ex.formats[0];
      this.cm.setSource(JSON.stringify(ex.doc, null, 2));
      if (this.mode === "export") {
        await this.checkFormats();
        await this.run();
      }
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

    clearViewer() {
      this.viewerTitle = "";
      this.cm.setViewer("");
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
        this.clearViewer();
        return;
      }
      this.viewerTitle = this.labels[out.format] || out.format;
      this.cm.setViewer(out.text, "xml");
      this.success++;
    },

    async import() {
      const out = await this.post("/v0/import", this.cm.input.state.doc.toString());
      if (!out) {
        this.clearViewer();
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
