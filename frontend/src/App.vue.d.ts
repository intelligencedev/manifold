import type { Node, Edge, Connection, NodeChange, EdgeChange } from '@vue-flow/core';
import type { Ref } from 'vue';

// Define the structure of the props.
interface AppProps {
    // No specific props for App.vue
}

// Define the structure of the emits:
interface AppEmits {
    // No specific emits for App.vue
}
// Declare the component itself:
declare const App: import('vue').DefineComponent<
    AppProps,
    {},
    {},
    {},
    {},
    {},
    {},
    AppEmits
>;

export default App;