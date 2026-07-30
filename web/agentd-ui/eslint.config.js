import vue from "eslint-plugin-vue";
import typescriptParser from "@typescript-eslint/parser";

export default [
  ...vue.configs["flat/essential"],
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: "latest",
      parser: typescriptParser,
      sourceType: "module",
    },
  },
  {
    files: ["**/*.vue"],
    languageOptions: {
      parserOptions: {
        parser: typescriptParser,
      },
    },
    rules: {
      "vue/multi-word-component-names": "off",
    },
  },
];
