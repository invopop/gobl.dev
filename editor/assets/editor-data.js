// editor-data.js -- Alpine.js data components for the GOBL editor.
// Loaded as a synchronous script (before Alpine.js which is deferred)
// so the alpine:init listener is registered in time.
document.addEventListener("alpine:init", () => {
  Alpine.data("editor", () => ({
    loading: false,
    envelop: false,
    error: null,
    // Counter so each successful build creates a fresh FlashMessage.
    success: 0,
    // Key of the format to export the built document into, if any.
    format: localStorage.getItem("editor-format") || "",
    exporting: false,

    init() {
      window.addEventListener("keydown", (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
          e.preventDefault();
          this.build();
        }
      });
      this.$watch("format", (v) => localStorage.setItem("editor-format", v));
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
        await this.export(result);
      } catch (e) {
        this.error = { message: e.message };
      } finally {
        this.loading = false;
      }
    },

    // exportCurrent exports the document in the editor, as it is, into the
    // selected format.
    async exportCurrent() {
      const ed = window._cmEditor;
      if (!ed || !this.format) return;
      this.error = null;
      try {
        await this.export(JSON.parse(ed.state.doc.toString()));
      } catch (e) {
        this.error = { message: e.message };
      }
    },

    // export converts the document or envelope into the selected format
    // and shows the result in the viewer pane. Errors are shown in the
    // error panel in the same shape as build errors.
    async export(doc) {
      if (!this.format || !window._cmSetViewer) return;
      this.exporting = true;
      try {
        const res = await fetch(
          "/v0/export?format=" + encodeURIComponent(this.format),
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(doc),
          },
        );
        if (!res.ok) {
          this.error = await res.json().catch(() => ({
            message: "export failed: " + res.status,
          }));
          window._cmSetViewer("");
          return;
        }
        window._cmSetViewer(await res.text());
      } finally {
        this.exporting = false;
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
