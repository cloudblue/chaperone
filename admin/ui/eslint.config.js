import pluginVue from "eslint-plugin-vue";
import configPrettier from "eslint-config-prettier";

export default [
  { ignores: ["dist/**"] },
  ...pluginVue.configs["flat/recommended"],
  configPrettier,
  {
    rules: {
      "semi": ["error", "always"],
      "vue/multi-word-component-names": "off",
    },
  },
];
