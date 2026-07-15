# Low-Poly Woodland Fox Rig-Ready Revision

## Approved design

Clean the Blender scene down to the fox model only. Delete the default cube, all cameras, all lights, the ground patch, and the ground tufts.

Fuse the structural anatomy—torso, neck, head, muzzle, ears, chest, legs, paws, and tail—into one watertight flat-shaded `Fox_Skinned` mesh using a voxel remesh. Keep `Fox_Eye_L`, `Fox_Eye_R`, and `Fox_Nose` as separate meshes for facial controls and material independence. Preserve `Fox_Root` as the future rig/animation parent.

## Acceptance criteria

- The scene contains only `Fox_Root`, `Fox_Skinned`, `Fox_Eye_L`, `Fox_Eye_R`, and `Fox_Nose`.
- `Fox_Skinned` is a single connected mesh object with flat shading.
- Eyes and nose remain separate meshes and are parented for later rigging.
- The updated `.blend` is saved at the existing workspace path.
