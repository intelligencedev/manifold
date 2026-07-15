# Low-Poly Woodland Fox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and save a stylized, flat-shaded low-poly woodland fox in a walking pose inside the connected Blender scene.

**Architecture:** Use one Blender Python procedure sent through the Blender MCP connection. It creates a dedicated `LowPolyFox` collection, a parented `Fox_Root`, reusable low-resolution primitive helpers, a warm woodland material palette, and a compact presentation setup. Existing scene objects remain untouched; only new namespaced objects are created or replaced when the procedure is rerun.

**Tech Stack:** Blender `bpy` Python API through `mcp__blender__execute_blender_code`, low-resolution mesh primitives, flat shading, Eevee/Cycles-compatible materials, and `.blend` scene saving.

## Global Constraints

- Preserve unrelated existing Blender objects and collections.
- Use faceted low-poly geometry with flat shading; do not add rigging, keyframes, fur simulation, textures, or a detailed forest environment.
- Use clearly named mesh objects under a dedicated `LowPolyFox` collection and parent them to `Fox_Root`.
- Use the agreed palette: orange body, dark brown-red accents, cream chest and muzzle, and dark eyes and nose.
- Save the finished scene as `/Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2/manifold/low_poly_woodland_fox.blend`.

---

### Task 1: Inspect the connected Blender scene

**Files:**
- Create: `/Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2/manifold/low_poly_woodland_fox.blend` (created in Task 5)

**Interfaces:**
- Consumes: the currently connected Blender scene.
- Produces: a structured scene summary containing scene name, object names/types, collections, active camera, and render engine.

- [ ] **Step 1: Query the scene without changing it**

Run `mcp__blender__execute_blender_code` with code equivalent to:

```python
import bpy

result = {
    "scene": bpy.context.scene.name,
    "objects": [
        {"name": obj.name, "type": obj.type}
        for obj in bpy.context.scene.objects
    ],
    "collections": [collection.name for collection in bpy.data.collections],
    "camera": bpy.context.scene.camera.name if bpy.context.scene.camera else None,
    "engine": bpy.context.scene.render.engine,
}
```

- [ ] **Step 2: Confirm namespaced rerun behavior**

Use the returned summary to confirm that any existing `LowPolyFox` collection can be replaced without touching unrelated collections. If it exists, the build procedure must remove only objects linked to that collection and then remove the empty collection before recreating it.

---

### Task 2: Create the fox collection, root, materials, and construction helpers

**Files:**
- Modify: the connected Blender scene through `mcp__blender__execute_blender_code`

**Interfaces:**
- Consumes: the scene summary from Task 1.
- Produces: `LowPolyFox`, `Fox_Root`, and the materials `Fox_Orange`, `Fox_DarkAccent`, `Fox_Cream`, `Fox_Dark`, `Fox_Ground`, and `Fox_GroundDark`.

- [ ] **Step 1: Recreate only the namespaced collection**

Create a new collection named `LowPolyFox`, link it to the scene collection, and create an Empty named `Fox_Root` inside it. If a prior `LowPolyFox` exists, remove its objects and collection only.

- [ ] **Step 2: Define reusable primitive helpers**

Implement helpers with these exact responsibilities:

```python
def make_material(name, color, roughness=0.9):
    """Create or replace a simple Principled BSDF material."""

def add_ico(name, location, scale, material, subdivisions=1, parent=None):
    """Add a low-resolution ico sphere, scale it, flat-shade it, and parent it."""

def add_cone(name, location, radius1, radius2, depth, rotation, material, vertices=6, parent=None):
    """Add a faceted cone/cylinder, flat-shade it, and parent it."""

def add_custom_mesh(name, vertices, faces, material, parent=None):
    """Create and link a mesh from explicit vertices/faces, then flat-shade it."""
```

Each created object must be linked to `LowPolyFox`, assigned exactly one intended material, and parented to `Fox_Root` when `parent` is provided.

- [ ] **Step 3: Verify helper output**

Return a result dictionary containing the collection name, root name, and the six material names. Stop and correct the procedure if any required name is missing.

---

### Task 3: Build the walking fox anatomy

**Files:**
- Modify: the connected Blender scene through `mcp__blender__execute_blender_code`

**Interfaces:**
- Consumes: the collection, root, materials, and helpers from Task 2.
- Produces: named mesh parts with a readable walking silhouette.

- [ ] **Step 1: Add the torso and neck**

Create `Fox_Torso` as an orange ico sphere scaled approximately `(1.55, 0.62, 0.78)` and `Fox_Neck` as a short orange faceted connector. Use the fox’s forward direction as positive X and place the body center around world `(0, 0, 1.35)`.

- [ ] **Step 2: Add head, muzzle, ears, eyes, nose, and chest**

Create:

```text
Fox_Head       orange low-poly ico sphere, raised and forward of torso
Fox_Muzzle     cream tapered cone or custom wedge
Fox_Ear_L      orange/dark pointed cone pair
Fox_Ear_R      orange/dark pointed cone pair
Fox_Eye_L      dark small ico sphere
Fox_Eye_R      dark small ico sphere
Fox_Nose       dark small low-poly cone or ico sphere
Fox_Chest      cream shallow low-poly patch on the front of the torso
```

Aim the head roughly 10 degrees upward and the muzzle forward. Place the eyes symmetrically on the front-facing sides of the head and keep the nose at the muzzle tip.

- [ ] **Step 3: Add four offset walking legs and paws**

Create these exact names:

```text
Fox_Leg_FrontL, Fox_Paw_FrontL
Fox_Leg_FrontR, Fox_Paw_FrontR
Fox_Leg_BackL,  Fox_Paw_BackL
Fox_Leg_BackR,  Fox_Paw_BackR
```

Use tapered faceted cones or custom low-poly prisms. Offset the legs into a diagonal walking gait: front-left and back-right slightly forward, front-right and back-left slightly back. Use dark accent material for the lower legs and paws, keep all paws near the same ground height, and avoid intersections with the torso.

- [ ] **Step 4: Add the tail and tail tip**

Create `Fox_Tail` as a segmented, tapered faceted chain or custom angular mesh sweeping backward from the rump with a slight upward curve. Add `Fox_TailTip` in cream or a lighter orange. Keep the tail large enough to reinforce the fox silhouette without obscuring the walking legs.

- [ ] **Step 5: Inspect the anatomy result**

Return object names grouped by anatomy and verify that every created fox mesh is in `LowPolyFox`, has flat shading, and is parented to `Fox_Root`. Check that at least 18 named fox mesh objects exist, including all required body, face, leg, paw, tail, and ground parts.

---

### Task 4: Add the woodland presentation setup

**Files:**
- Modify: the connected Blender scene through `mcp__blender__execute_blender_code`

**Interfaces:**
- Consumes: the completed fox anatomy from Task 3 and the inspected scene camera/render state.
- Produces: `Fox_GroundPatch`, optional `Fox_GroundTuft_*` meshes, a usable camera framing the asset, and lighting if no suitable setup exists.

- [ ] **Step 1: Add the ground patch**

Create `Fox_GroundPatch` as a flat, low-resolution irregular polygon or flattened ico sphere beneath the fox. Use `Fox_Ground` and add two or three small `Fox_GroundTuft_01`-style faceted wedges in `Fox_GroundDark` to suggest woodland without creating a full environment.

- [ ] **Step 2: Preserve or create preview camera and lights**

If the inspected scene already has a camera and usable lights, leave them unchanged. Otherwise create namespaced `Fox_Camera`, `Fox_KeyLight`, and `Fox_FillLight`, with a three-quarter view from positive Y and slightly positive Z looking toward the fox root.

- [ ] **Step 3: Verify the visual staging**

Confirm the fox is fully inside the camera frame, the paws sit on the ground patch, and the materials are visible under the active render engine. Keep the background neutral and avoid modifying existing world settings unless the scene has no preview setup.

---

### Task 5: Save and verify the finished Blender asset

**Files:**
- Create: `/Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2/manifold/low_poly_woodland_fox.blend`

**Interfaces:**
- Consumes: the staged `LowPolyFox` collection.
- Produces: a saved `.blend` file and an inspection report.

- [ ] **Step 1: Save the scene**

Run:

```python
bpy.ops.wm.save_as_mainfile(
    filepath="/Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2/manifold/low_poly_woodland_fox.blend"
)
```

- [ ] **Step 2: Reinspect the saved scene**

Return:

```python
result = {
    "filepath": bpy.data.filepath,
    "collection_objects": sorted(obj.name for obj in bpy.data.collections["LowPolyFox"].objects),
    "materials": sorted({slot.material.name for obj in bpy.data.collections["LowPolyFox"].objects for slot in obj.material_slots if slot.material}),
    "root_children": sorted(obj.name for obj in bpy.data.objects if obj.parent and obj.parent.name == "Fox_Root"),
}
```

- [ ] **Step 3: Check acceptance criteria**

The build is accepted only if the filepath matches the required path, `LowPolyFox` exists, `Fox_Root` has children, the body and face materials are present, the four walking legs and paws are present, and no unrelated pre-existing object names were removed.

---

### Task 6: Clean the scene and create the rig-ready continuous body

**Files:**
- Modify: the connected Blender scene through `mcp__blender__execute_blender_code`
- Modify: `/Users/arturoaquino/Documents/manifold-tmp/users/0/projects/3eb68163-63b9-4957-bb56-415b63ceb5c2/manifold/low_poly_woodland_fox.blend`

**Interfaces:**
- Consumes: the saved `LowPolyFox` build from Task 5.
- Produces: one watertight `Fox_Skinned` body mesh plus separate `Fox_Eye_L`, `Fox_Eye_R`, and `Fox_Nose` meshes.

- [ ] **Step 1: Delete non-model objects**

Delete every scene object that is not part of the fox model, including the ground patch, ground tufts, cameras, lights, and default starter objects. Keep only fox anatomy, `Fox_Root`, and the three separate facial detail meshes.

- [ ] **Step 2: Voxel-remesh the structural anatomy**

Join torso, neck, head, muzzle, ears, chest, upper/lower legs, paws, tail, and tail tip into one selected mesh. Apply transforms, add a Voxel Remesh modifier with `voxel_size=0.06`, apply it, recalculate outside normals, and flat-shade the resulting `Fox_Skinned` mesh. Keep the three facial detail meshes out of the join and parent them to `Fox_Root`.

- [ ] **Step 3: Verify and save**

Confirm that the scene contains only `Fox_Root`, `Fox_Skinned`, `Fox_Eye_L`, `Fox_Eye_R`, and `Fox_Nose`; confirm the body has one connected mesh component and no active camera or lights; then save the updated `.blend` file.
