import plantCellRender from '../assets/cell-plant-render.png'

export const CELL_TYPES = [
  { id: 'plant', name: 'Plant Specimen', type: 'Starter Asset', accent: '#82b366' },
  { id: 'white-blood', name: 'Immune Specimen', type: 'Starter Asset', accent: '#7e6edb' },
  { id: 'neuron', name: 'Neuron Specimen', type: 'Starter Asset', accent: '#8b5cf6' },
  { id: 'epithelial', name: 'Tissue Specimen', type: 'Starter Asset', accent: '#e07a7a' },
  { id: 'bacteria', name: 'Microbe Specimen', type: 'Starter Asset', accent: '#5fbf9f' },
  { id: 'animal', name: 'Organic Specimen', type: 'Starter Asset', accent: '#459ccf' },
  { id: 'muscle', name: 'Fiber Specimen', type: 'Starter Asset', accent: '#d25762' },
]

export const SEEDED_GENERATED_CELLS = [
  {
    id: 'tripo-epithelial-test',
    name: 'Tripo Tissue Test',
    type: 'Cached AI Asset',
    accent: '#e07a7a',
    custom: true,
    template: 'epithelial',
    imageUrl: '/epithelial_cell_3d_tripo_input.png',
    generation: {
      provider: 'tripo',
      status: 'success',
      taskId: 'dc44beb1-e1a1-4650-9337-fbe418b7b154',
      modelUrl: '/generated-models/tripo-epithelial-cell-test.glb',
      rawModelUrl: '',
      message: 'Cached GLB from the verified Tripo epithelial test run.',
    },
  },
  {
    id: 'tripo-plant-test',
    name: 'Tripo Plant Test',
    type: 'Cached AI Asset',
    accent: '#82b366',
    custom: true,
    template: 'plant',
    imageUrl: plantCellRender,
    generation: {
      provider: 'tripo',
      status: 'success',
      taskId: '1db80a91-e202-4494-b17b-147de74cae81',
      modelUrl: '/generated-models/tripo-plant-cell-test.glb',
      rawModelUrl: '',
      message: 'Cached GLB from the verified Tripo test run.',
    },
  },
]

export const KHRONOS_REFERENCE_CELLS = [
  {
    id: 'khronos-transmission-test',
    name: 'Transmission Test',
    type: 'Khronos PBR Reference',
    accent: '#72a4bf',
    custom: true,
    reference: true,
    template: 'animal',
    imageUrl: 'https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Models/main/2.0/TransmissionTest/screenshot/screenshot_large.png',
    referenceSummary: 'Official Khronos glTF sample for KHR_materials_transmission. Useful for tuning transparent membranes, glassy shells, and opacity interactions.',
    referenceLicense: 'CC0, Adobe via Khronos glTF Sample Models',
    referenceSource: 'https://github.com/KhronosGroup/glTF-Sample-Models/tree/main/2.0/TransmissionTest',
    generation: {
      provider: 'reference',
      requestedProvider: 'reference',
      status: 'success',
      taskId: 'khronos-transmission-test',
      modelUrl: 'https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Models/main/2.0/TransmissionTest/glTF-Binary/TransmissionTest.glb',
      rawModelUrl: '',
      message: 'Remote Khronos GLB reference for transparent material behavior.',
    },
  },
  {
    id: 'khronos-transmission-roughness',
    name: 'Transmission Roughness',
    type: 'Khronos PBR Reference',
    accent: '#8eb4cf',
    custom: true,
    reference: true,
    template: 'animal',
    imageUrl: 'https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Models/main/2.0/TransmissionRoughnessTest/screenshot/screenshot-large.png',
    referenceSummary: 'Official Khronos glTF sample for transmission, IOR, roughness, and volume. Useful for soft translucent shells and layered material haze.',
    referenceLicense: 'CC-BY 4.0, Ed Mackey / Analytical Graphics via Khronos glTF Sample Models',
    referenceSource: 'https://github.com/KhronosGroup/glTF-Sample-Models/tree/main/2.0/TransmissionRoughnessTest',
    generation: {
      provider: 'reference',
      requestedProvider: 'reference',
      status: 'success',
      taskId: 'khronos-transmission-roughness',
      modelUrl: 'https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Models/main/2.0/TransmissionRoughnessTest/glTF-Binary/TransmissionRoughnessTest.glb',
      rawModelUrl: '',
      message: 'Remote Khronos GLB reference for IOR and translucent roughness.',
    },
  },
  {
    id: 'khronos-mosquito-amber',
    name: 'Mosquito In Amber',
    type: 'Khronos Bio Reference',
    accent: '#d18a42',
    custom: true,
    reference: true,
    template: 'bacteria',
    imageUrl: 'https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Models/main/2.0/MosquitoInAmber/screenshot/screenshot.jpg',
    referenceSummary: 'Biological specimen in a transparent amber volume. Useful as a target for organic detail plus translucent material presentation.',
    referenceLicense: 'CC-BY 4.0, Loic Norgeot / Geoffrey Marchal / Sketchfab via Khronos glTF Sample Models',
    referenceSource: 'https://github.com/KhronosGroup/glTF-Sample-Models/tree/main/2.0/MosquitoInAmber',
    generation: {
      provider: 'reference',
      requestedProvider: 'reference',
      status: 'success',
      taskId: 'khronos-mosquito-amber',
      modelUrl: 'https://raw.githubusercontent.com/KhronosGroup/glTF-Sample-Models/main/2.0/MosquitoInAmber/glTF-Binary/MosquitoInAmber.glb',
      rawModelUrl: '',
      message: 'Remote Khronos biological GLB reference. This model is larger and may take longer to load.',
    },
  },
]

export const ORGANELLES = {
  nucleus: {
    label: 'Core Volume',
    title: 'Core Volume',
    subtitle: 'Main internal mass',
    size: 'Dominant inner volume',
    location: 'Near the visual center',
    visible: 'Visible in inspection mode',
    note: 'The core volume is the main readable mass of the model. It helps the viewer understand scale, silhouette, and depth.',
    accent: '#7b4bb4',
  },
  lysosome: {
    label: 'Accent Features',
    title: 'Accent Features',
    subtitle: 'Secondary forms and visual anchors',
    size: 'Small supporting features',
    location: 'Distributed across the model',
    visible: 'Visible as detail clusters',
    note: 'Accent features make the generated object feel less flat. They are useful for checking whether the source image produced meaningful 3D detail.',
    accent: '#8d58b8',
  },
  mitochondria: {
    label: 'Material Bands',
    title: 'Material Bands',
    subtitle: 'Color and surface-material zones',
    size: 'Medium surface groups',
    location: 'Across the visible surface',
    visible: 'Visible through material contrast',
    note: 'Material bands show whether the model preserves color separation, roughness, and recognisable surface regions from the reference.',
    accent: '#df7046',
  },
  membrane: {
    label: 'Surface Shell',
    title: 'Surface Shell',
    subtitle: 'Outer silhouette and contact layer',
    size: 'Full object boundary',
    location: 'Model perimeter',
    visible: 'Always visible',
    note: 'The surface shell is the most important quality signal. If the silhouette reads correctly, the model will work better for screenshots and demos.',
    accent: '#7aa4bf',
  },
  granules: {
    label: 'Detail Points',
    title: 'Detail Points',
    subtitle: 'Small high-frequency geometry',
    size: 'Fine detail marks',
    location: 'Surface and recessed areas',
    visible: 'Visible when zoomed in',
    note: 'Detail points reveal whether the generator created real dimensional cues rather than a smooth blob or flat card.',
    accent: '#5b82c4',
  },
}

export const ORGANELLE_ORDER = ['nucleus', 'lysosome', 'mitochondria', 'membrane', 'granules']

export const MICROSCOPE_IMAGES = [
  { label: 'Studio Preview', tone: 'light', note: 'Clean presentation view for screenshots.' },
  { label: 'Texture Pass', tone: 'purple', note: 'Material and color separation preview.' },
  { label: 'Depth Preview', tone: 'mono', note: 'Shape and relief readability preview.' },
]

export const WORKSPACE_PANELS = {
  Gallery: 'Saved render angles, thumbnails, and exported presentation shots.',
  Library: 'Starter models, generated assets, local imports, and reference GLB files.',
  Notebooks: 'Observation notes linked to the selected asset and inspection part.',
  Logs: 'Diagnostics, API request logs, and generation troubleshooting.',
  Settings: 'Viewer quality, provider defaults, screenshot size, and export preferences.',
  Compare: 'Side-by-side model comparison for shape, material, and generation quality.',
  Profile: 'Current workspace: cc2cx AI3D生成.',
}

export const CELL_PROFILES = {
  plant: {
    summary: 'Rigid wall, large vacuole, chloroplast-like structures, Golgi stacks, and a clear nucleus.',
    occurs: 'Leaves, stems, roots, and photosynthetic tissue.',
    comparison: 'Has a rigid wall and chloroplast-like organelles; animal cells do not.',
    compareTarget: 'animal',
    organelles: ['membrane', 'nucleus', 'mitochondria', 'granules'],
  },
  'white-blood': {
    summary: 'Soft immune cell with lobed nucleus, many lysosomes, granules, and deformable membrane.',
    occurs: 'Blood, lymph, and inflamed tissue.',
    comparison: 'More mobile and granular than epithelial cells; built for immune response.',
    compareTarget: 'epithelial',
    organelles: ['lysosome', 'nucleus', 'mitochondria', 'membrane', 'granules'],
  },
  neuron: {
    summary: 'Compact soma with branching dendrite and axon-like extensions for signal routing.',
    occurs: 'Brain, spinal cord, and peripheral nerves.',
    comparison: 'Long membrane extensions dominate the shape; most other cells stay compact.',
    compareTarget: 'muscle',
    organelles: ['membrane', 'nucleus', 'mitochondria', 'granules'],
  },
  epithelial: {
    summary: 'Sheet-like tissue cell with apical ridges, junction cues, membrane boundaries, and nucleus.',
    occurs: 'Skin, ducts, organ linings, and protective surfaces.',
    comparison: 'Designed for barrier tissue, unlike free-moving white blood cells.',
    compareTarget: 'white-blood',
    organelles: ['membrane', 'nucleus', 'mitochondria', 'granules'],
  },
  bacteria: {
    summary: 'Prokaryotic capsule with nucleoid DNA, ribosome dots, pili, and a flagellum cue.',
    occurs: 'Soil, water, gut flora, skin, and many environmental surfaces.',
    comparison: 'No nucleus or membrane-bound organelles; the DNA sits in a nucleoid region.',
    compareTarget: 'animal',
    organelles: ['membrane', 'granules'],
  },
  animal: {
    summary: 'Flexible eukaryotic cell with nucleus, mitochondria, vesicles, and soft membrane.',
    occurs: 'Organs, connective tissue, blood-related tissues, and cultured samples.',
    comparison: 'Lacks the rigid wall shown in plant cells.',
    compareTarget: 'plant',
    organelles: ['membrane', 'nucleus', 'mitochondria', 'lysosome', 'granules'],
  },
  muscle: {
    summary: 'Elongated fiber-like cell with striation cues and extra mitochondria for contraction.',
    occurs: 'Skeletal muscle, cardiac tissue, and contractile tissue samples.',
    comparison: 'Elongated and energy-heavy compared with round animal cells.',
    compareTarget: 'neuron',
    organelles: ['membrane', 'nucleus', 'mitochondria', 'granules'],
  },
}

export const DEFAULT_ORGANELLE_BY_CELL = {
  plant: 'membrane',
  'white-blood': 'lysosome',
  neuron: 'nucleus',
  epithelial: 'membrane',
  bacteria: 'granules',
  animal: 'nucleus',
  muscle: 'mitochondria',
}

export const CELL_DETAIL_OVERRIDES = {
  plant: {
    nucleus: {
      subtitle: 'The command center',
      size: '5-10 um in diameter',
      location: 'Usually central',
      visible: 'Yes',
      note: 'The nucleus is surrounded by a double membrane called the nuclear envelope, which contains pores that regulate the movement of molecules in and out.',
      funFact: 'The nucleus was one of the first cell structures discovered.',
    },
    membrane: {
      title: 'Cell Wall',
      subtitle: 'Rigid outer support',
      size: 'About 0.1-10 um thick',
      location: 'Outer boundary',
      visible: 'Yes',
      note: 'Plant cells have a rigid wall outside the membrane. It gives the cell shape and helps resist pressure from the large central vacuole.',
      funFact: 'Cellulose fibers make plant cell walls strong and flexible.',
    },
    mitochondria: {
      note: 'Mitochondria convert stored sugars into usable energy for growth, repair, and transport inside the plant cell.',
      funFact: 'Plant cells have both mitochondria and chloroplasts.',
    },
    granules: {
      title: 'Golgi Apparatus',
      subtitle: 'Packaging and transport',
      note: 'The Golgi modifies, sorts, and packages proteins and lipids before they move to their next destination.',
      funFact: 'Golgi stacks look like folded ribbons in many educational renders.',
    },
  },
  'white-blood': {
    lysosome: {
      note: 'White blood cells carry many lysosomes because they digest captured particles and damaged material during immune response.',
      funFact: 'The clustered purple granules are emphasized here so they remain readable while rotating.',
    },
    nucleus: {
      note: 'The lobed nucleus is a key visual feature of many immune cells and helps the cell deform through narrow tissue gaps.',
    },
  },
  neuron: {
    membrane: {
      title: 'Axon and Dendrites',
      subtitle: 'Signal-routing branches',
      location: 'Extending from the soma',
      note: 'Neurons depend on long membrane extensions to receive and transmit electrical signals across large distances.',
      funFact: 'The branching structure matters more visually than a perfectly round cell body.',
    },
  },
  epithelial: {
    membrane: {
      title: 'Apical Surface',
      subtitle: 'Barrier and contact layer',
      location: 'Tissue-facing edge',
      note: 'Epithelial cells form sheets. The surface ridges and junction lines make that tissue architecture visible.',
    },
  },
  bacteria: {
    granules: {
      title: 'Nucleoid and Ribosomes',
      subtitle: 'Prokaryotic core material',
      size: 'Not membrane bound',
      location: 'Central cytoplasm',
      note: 'Bacteria do not have a nucleus. The blue DNA coil and small ribosome dots represent the prokaryotic interior.',
      funFact: 'The flagellum and pili are exaggerated for readability in the 3D viewer.',
    },
  },
  animal: {
    nucleus: {
      note: 'Animal cells are shown with a softer membrane, central nucleus, mitochondria, and transport structures without a rigid wall.',
    },
  },
  muscle: {
    mitochondria: {
      note: 'Muscle fibers contain many mitochondria because contraction needs sustained ATP production.',
      funFact: 'The stripe pattern is a simplified sarcomere cue, not a literal molecular model.',
    },
  },
}

export const CELL_BODY = {
  plant: { color: '#b8d983', scale: [1.38, 1.04, 0.76], kind: 'box' },
  'white-blood': { color: '#c9d3e6', scale: [1.34, 1.18, 0.92], kind: 'sphere' },
  neuron: { color: '#d8c6ff', scale: [0.78, 0.68, 0.58], kind: 'sphere' },
  epithelial: { color: '#efb4a6', scale: [1.22, 0.92, 0.52], kind: 'box' },
  bacteria: { color: '#8ed9bc', scale: [0.9, 1, 0.56], kind: 'capsule' },
  animal: { color: '#b8dcf2', scale: [1.18, 1.08, 0.9], kind: 'sphere' },
  muscle: { color: '#e78a94', scale: [0.82, 1.1, 0.48], kind: 'capsule' },
}
