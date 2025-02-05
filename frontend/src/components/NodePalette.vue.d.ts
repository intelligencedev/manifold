// Define the props for NodePalette
interface NodePaletteProps {
    // No specific props for NodePalette
}

// Define the emits for NodePalette:
interface NodePaletteEmits {
    (e: 'drag-start', payload: { event: DragEvent; type: string }): void;
}


// Declare the component
declare const NodePalette: import('vue').DefineComponent<
    NodePaletteProps,
    {},
    {},
    {},
    {},
    {},
    {},
    NodePaletteEmits
>;

export default NodePalette;