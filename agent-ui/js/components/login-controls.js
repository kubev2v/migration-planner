export default class LoginBtn {
    constructor(props = {}) {
        this.props = props;
    }

    render() {
        let self = this;
        let elem = document.createElement('div');

        elem.innerHTML = `
            <button id="login-btn" class="pf-v6-c-button pf-m-primary" type="submit" ${self.props.isButtonEnabled() ? "" : "disabled"}>
                <span class="pf-v6-c-button__text">Log In</span>
            </button>
        `;

        if (self.props.spinnerEnabled) {
            elem.innerHTML = `
                ${elem.innerHTML}
                <div>
                    <svg class="pf-v6-c-spinner" role="progressbar" viewBox="0 0 100 100" aria-label="Loading...">
                        <circle class="pf-v6-c-spinner__path" cx="50" cy="50" r="25" fill="none" />
                    </svg>
                    <p>${self.props.msg}</p>
                </div>
            `;
        }

        elem.querySelector("button").addEventListener('click', () => {
            self.props.postCallback();
        });

        return elem;
    }
}
