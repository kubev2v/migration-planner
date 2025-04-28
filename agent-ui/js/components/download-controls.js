export default class DownloadControls {
    constructor(props = {}) {
        this.props = props;
    }

    render() {
        let self = this;
        let elem = document.createElement('div');
        elem.className = "download-controls";

        elem.innerHTML = `
            <button id="btn-assessment" class="pf-v6-c-button pf-m-primary">
                <span class="pf-v6-c-button__text">Go back to assessment wizard</span>
            </button>
            <button id="btn-download" class="pf-v6-c-button pf-m-secondary">
                <span class="pf-v6-c-button__text">Download inventory</span>
            </button>
        `;

        elem.querySelector("#btn-download").addEventListener('click', () => {
            self.props.downloadCallback();
        });

        elem.querySelector("#btn-assessment").addEventListener('click', () => {
            self.props.goBackBtnCallback();
        });

        return elem;
    }
}
