// happy-dom gives the unit tests a real DOM without a browser: the built-ins in
// utils/page.ts are ordinary functions over document/window, so they can be
// tested directly instead of only through a live tab.
import { GlobalRegistrator } from '@happy-dom/global-registrator';

GlobalRegistrator.register({ url: 'https://example.test/' });
