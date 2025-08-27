

const barWidth = 2;
const barSpacing = 4;


function map_range(value: number, low1: number, high1: number, low2: number, high2: number) {
    return low2 + (high2 - low2) * (value - low1) / (high1 - low1);
}


export class FillChart {
    element: HTMLCanvasElement;

    ctx: CanvasRenderingContext2D
    data: number[]
    w: number
    h: number

    constructor(id: string, data: number[]) {
        this.element = document.getElementById(id) as HTMLCanvasElement || {} as HTMLCanvasElement

        this.data = data
        let ctx = this.element.getContext("2d") || {} as CanvasRenderingContext2D
        this.w = ctx.canvas.clientWidth*2
        this.h = ctx.canvas.clientHeight*2

        ctx.scale(0.5,0.5)
        this.ctx = ctx
        this.draw()
    }

    public pickColor(index: number): string {
        const colors: string[] = ["0,128,255", "48,209,88", "255,128,12"]
        let idx: number = index % colors.length
        return colors[idx];
    }

    draw() {
        let ctx = this.ctx
        this.w = this.ctx.canvas.width*2
        this.h = this.ctx.canvas.height*2
        let w = this.w
        let h = this.h

        ctx.clearRect(0, 0, w, h);
        ctx.beginPath();
        ctx.lineWidth = 2
        ctx.lineJoin = "round"


        // ctx.strokeStyle = "rgba(255,255,255,0.25)";
        let sum = 1;
        // this.data.forEach(d => sum+=d)
        let last = 0;
        // let minY = Math.min(...this.data);
        // let maxY = Math.max(...this.data);

        let getX = (index: number): number => {
            return last;
        }

        let getW = (index: number): number => {
            return map_range(this.data[index], 0, sum, 0, w)
        }

        for (let i = 0; i < this.data.length; i++) {
            let r = Math.random()*255;
            let g = Math.random()*255;
            ctx.strokeStyle = `rgba(${this.pickColor(i)}, 1)`
            ctx.fillStyle = `rgba(${this.pickColor(i)}, 0.8)`
            let x = getX(i);
            let w = getW(i)


            ctx.fillRect(x, 0, w, h)
            last = x+w
        }



    }

}
