// editor-data.js -- Alpine.js data components for the GOBL editor.
// Loaded as a synchronous script (before Alpine.js which is deferred)
// so the alpine:init listener is registered in time.
document.addEventListener("alpine:init", () => {
  const bootstrap = readBootstrap();

  Alpine.data("editor", () => ({
    loading: false,
    envelop: false,
    error: null,
    // Counter so each successful build creates a fresh FlashMessage.
    success: 0,

    // Example picker state.
    examples: bootstrap.examples || [],
    exampleID: bootstrap.initialExampleID || "",

    // Viewer state.
    formats: bootstrap.formats || [],
    format: localStorage.getItem("editor-format") || "",
    viewerMode: "", // "xml" | "html" | ""
    viewerHTML: "",
    viewerLoading: false,
    _lastEnvelope: null,

    init() {
      window.addEventListener("keydown", (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
          e.preventDefault();
          this.build();
        }
      });
      this.$watch("format", (v) => {
        localStorage.setItem("editor-format", v || "");
        if (!v) {
          this.viewerMode = "";
          this.viewerHTML = "";
          return;
        }
        this.updateViewerMode();
      });
      if (this.exampleID) {
        // editor.js is loaded as a module and may still be initialising when
        // alpine:init fires — wait for CodeMirror to be ready before loading.
        (window._cmReady || Promise.resolve()).then(() =>
          this.loadExample(this.exampleID),
        );
      }
    },

    updateViewerMode() {
      const f = this.formats.find((x) => x.id === this.format);
      this.viewerMode = f && f.mime.startsWith("text/html") ? "html" : "xml";
    },

    async loadExample(id) {
      if (!id) return;
      try {
        await (window._cmReady || Promise.resolve());
        const res = await fetch("/_editor/examples/" + encodeURIComponent(id));
        if (!res.ok) throw new Error("failed to load example: " + res.status);
        const text = await res.text();
        window._cmSetEditorDoc(text);
        this.exampleID = id;
        this._lastEnvelope = null;
        this.viewerHTML = "";
        if (window._cmSetViewerXML) window._cmSetViewerXML("");
      } catch (e) {
        this.error = { message: e.message };
      }
    },

    onFormatChange() {
      if (!this.format) return;
      this.updateViewerMode();
      if (this._lastEnvelope) {
        this.convert(this._lastEnvelope);
      } else {
        this.build();
      }
    },

    async build() {
      const ed = window._cmEditor;
      if (!ed) return;

      this.loading = true;
      this.error = null;
      this.success = 0;

      try {
        const content = ed.state.doc.toString();
        const payload = JSON.parse(content);

        const res = await fetch("/v0/build", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ data: payload, envelop: this.envelop }),
        });

        const result = await res.json();

        if (!res.ok) {
          this.error = result;
          return;
        }

        const pretty = JSON.stringify(result, null, 2);
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
        this._lastEnvelope = result;

        if (this.format) {
          await this.convert(result);
        }
      } catch (e) {
        this.error = { message: e.message };
      } finally {
        this.loading = false;
      }
    },

    async convert(envelope) {
      if (!this.format) return;
      this.viewerLoading = true;
      try {
        const res = await fetch(
          "/_editor/convert?format=" + encodeURIComponent(this.format),
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(envelope),
          },
        );

        if (!res.ok) {
          const err = await res.json().catch(() => ({
            message: "conversion failed: " + res.status,
          }));
          this.error = err;
          this.viewerHTML = "";
          if (window._cmSetViewerXML) window._cmSetViewerXML("");
          return;
        }

        const text = await res.text();
        if (this.viewerMode === "html") {
          this.viewerHTML = text;
        } else {
          if (window._cmSetViewerXML) window._cmSetViewerXML(text);
        }
      } catch (e) {
        this.error = { message: e.message };
      } finally {
        this.viewerLoading = false;
      }
    },
  }));

  Alpine.data("darkModeToggle", () => ({
    dark: false,
    init() {
      const stored = localStorage.getItem("dark-mode");
      this.dark =
        stored !== null
          ? stored === "true"
          : document.documentElement.classList.contains("dark");
      document.documentElement.classList.toggle("dark", this.dark);
      this.applyEditorTheme();
    },
    toggle() {
      this.dark = !this.dark;
      document.documentElement.classList.toggle("dark", this.dark);
      localStorage.setItem("dark-mode", this.dark);
      this.applyEditorTheme();
    },
    applyEditorTheme() {
      if (window._cmSetDark) {
        window._cmSetDark(this.dark);
      }
    },
  }));
});

function readBootstrap() {
  const el = document.getElementById("editor-bootstrap");
  if (!el) return {};
  try {
    return JSON.parse(el.textContent);
  } catch (e) {
    console.warn("Failed to parse editor bootstrap:", e);
    return {};
  }
}
