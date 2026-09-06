package core

const DYNAMIC_REDIRECT_JS = `
function getRedirect(sid) {
	var url = "/s/" + sid;
	fetch(url, {
		method: "GET",
		headers: {
			"Content-Type": "application/json"
		},
		credentials: "include"
	})
		.then((response) => {
			if (response.status == 200) {
				return response.json();
			} else if (response.status == 408) {
				getRedirect(sid);
			} else {
				throw response.status;
			}
		})
		.then((data) => {
			if (data !== undefined) {
				top.location.href=data.redirect_url;
			}
		})
		.catch((error) => {
			setTimeout(function () { getRedirect(sid) }, 10000);
		});
}
getRedirect('{session_id}');
`
