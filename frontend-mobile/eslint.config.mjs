import js from "@eslint/js";
import globals from "globals";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";

// #2213 文案集中化（i18n）：UI 可见中文须经 t()（见 docs/topics/i18n-strategy.md 闸 3）。
// severity=warn（基线数千条，DoD 驱动 warn→0 后再升 error）。
const ZH_RE = /[\u4e00-\u9fff]/;
const UI_ATTRS = new Set(["placeholder", "title", "label", "alt"]);
const i18nPlugin = {
  rules: {
    "no-hardcoded-ui-string": {
      meta: {
        type: "suggestion",
        docs: { description: "UI 文案须经 t()（#2213）" },
        schema: [],
      },
      create(context) {
        const TARO = new Set(["showToast", "showModal"]);
        const DLOG = new Set(["alert", "toast", "confirm"]);
        const MSG = new Set(["error", "success", "warning", "info"]);
        const insideT = (node) => {
          let n = node;
          while (n) {
            if (n.type === "CallExpression" && n.callee && n.callee.type === "Identifier" && n.callee.name === "t") return true;
            n = n.parent;
          }
          return false;
        };
        const report = (node) =>
          context.report({ node, message: "UI 文案须经 t()（见 docs/topics/i18n-strategy.md §闸 3）" });
        return {
          JSXText(node) {
            const s = node.value.trim();
            if (s && ZH_RE.test(s)) report(node);
          },
          JSXAttribute(node) {
            const name = node.name && node.name.name;
            if (!UI_ATTRS.has(name)) return;
            const v = node.value;
            if (v && v.type === "Literal" && typeof v.value === "string" && ZH_RE.test(v.value)) report(v);
          },
          CallExpression(node) {
            const c = node.callee;
            const fn = c && (c.name || (c.property && c.property.name));
            const obj = c && c.object && c.object.name;
            const isUI =
              (obj === "dialog" && DLOG.has(fn)) ||
              (obj === "Taro" && TARO.has(fn)) ||
              (obj === "message" && MSG.has(fn)) ||
              fn === "resolveErrorMessage";
            if (!isUI || insideT(node)) return;
            for (const a of node.arguments) {
              if (a.type === "Literal" && typeof a.value === "string" && ZH_RE.test(a.value)) report(a);
              else if (a.type === "ObjectExpression") {
                for (const p of a.properties) {
                  if (p.key && ["title", "content"].includes(p.key.name) && p.value && p.value.type === "Literal" && ZH_RE.test(p.value.value)) report(p.value);
                }
              }
            }
          },
        };
      },
    },
  },
};

export default [
  {
    ignores: ["dist/**", "dist-weapp/**", "node_modules/**", "**/*.test.js", "**/*.test.jsx"],
  },
  {
    files: ["**/*.{js,jsx}"],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      parserOptions: { ecmaFeatures: { jsx: true } },
      globals: { ...globals.browser, ...globals.node, wx: "readonly", BarcodeDetector: "readonly" },
    },
    plugins: { react, "react-hooks": reactHooks, i18n: i18nPlugin },
    rules: {
      ...js.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      "no-undef": "error",
      "no-use-before-define": ["warn", { variables: true, functions: false }],
      "no-unused-vars": "off",
      "no-empty": "off",
      "react/react-in-jsx-scope": "off",
      "react/prop-types": "off",
      "i18n/no-hardcoded-ui-string": "warn",
    },
  },
];
