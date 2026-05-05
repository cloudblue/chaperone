import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart } from 'echarts/charts';
import {
	GridComponent,
	TooltipComponent,
	LegendComponent,
	DataZoomComponent,
} from 'echarts/components';

use([
	CanvasRenderer,
	LineChart,
	GridComponent,
	TooltipComponent,
	LegendComponent,
	DataZoomComponent,
]);

export { default as VChart } from 'vue-echarts';
