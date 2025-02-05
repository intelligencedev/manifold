import type { NodeProps, HandleProps, Position } from '@vue-flow/core';
import type { Ref } from 'vue';

// Define the shape of the input prop:
interface AgentNodeData {
    type: 'AgentNode';
    labelStyle: {
        fontWeight: string;
    };
    hasInputs: boolean;
    hasOutputs: boolean;
    inputs: {
        endpoint: string;
        api_key: string;
        model: string;
        system_prompt: string;
        user_prompt: string;
    };
    outputs: {
        response: string;
    };
    models: string[];
    run?: () => Promise<any>; // Add run function to interface here
}
    
// Define the props for AgentNode
interface AgentNodeProps extends NodeProps {
    id: string;
    data: AgentNodeData;
}

// Define the emits for AgentNode
interface AgentNodeEmits {
    (e: 'update:data', payload: { id: string; data: AgentNodeData }): void;
    (e: 'resize', payload: any): void;
}

// Declare the component itself:
declare const AgentNode: import('vue').DefineComponent<
    AgentNodeProps,
    {},
    {},
    {},
    {},
    {},
    {},
    AgentNodeEmits
>;

export default AgentNode;