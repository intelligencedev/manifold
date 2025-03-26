// ui/src/composables/useLayout.js
import dagre from '@dagrejs/dagre';
import { Position, useVueFlow } from '@vue-flow/core';
import { ref } from 'vue';

/**
 * Composable to run the layout algorithm on the graph.
 * It uses the `dagre` library to calculate the layout of the nodes and edges.
 */
export default function useLayout() {
  const { fitView } = useVueFlow();  // if you want auto-fit after layout
  const graph = ref(new dagre.graphlib.Graph());
  const previousDirection = ref('LR');

  function layout(nodes, edges, direction) {
    const dagreGraph = new dagre.graphlib.Graph();
    graph.value = dagreGraph;

    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({
      rankdir: direction,
      ranksep: 100,
      nodesep: 50,
      edgesep: 50,
      marginx: 20,
      marginy: 20
    });

    previousDirection.value = direction;

    // **(Important)**: We rely on each node's .dimensions for the correct width/height.
    for (const node of nodes) {
      dagreGraph.setNode(node.id, {
        width: node.dimensions?.width || 150,
        height: node.dimensions?.height || 50,
      });
    }

    for (const edge of edges) {
      dagreGraph.setEdge(edge.source, edge.target);
    }

    dagre.layout(dagreGraph);

    const isHorizontal = direction === 'LR' || direction === 'RL';

    const newNodes = nodes.map((node) => {
      const dagrePosition = dagreGraph.node(node.id);
      return {
        ...node,
        position: { x: dagrePosition.x, y: dagrePosition.y },
        targetPosition: isHorizontal ? Position.Left : Position.Top,
        sourcePosition: isHorizontal ? Position.Right : Position.Bottom,
      };
    });

    // optional: auto-fit after the layout is updated
    // setTimeout(() => fitView({ duration: 500 }), 0);

    return newNodes;
  }

  return { graph, layout, previousDirection };
}
