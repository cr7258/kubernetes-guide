extern crate wasm_bindgen;
use wasm_bindgen::prelude::*;

#[wasm_bindgen]
extern {
    // console.log(xxx)
    #[wasm_bindgen(js_namespace=console)]
    fn log(s:&str);
}

macro_rules! echo {
    ($expr:expr) => {
        log(format!("{}", $expr).as_str());
    };
}

// wasm-bindgen ：wasm 模块和 JavaScript 之间进行交互的一个第三方库
#[wasm_bindgen]
pub fn echo(s: & str){
    // format!("{}","chengzw")
    echo!(s);
}

#[wasm_bindgen]
pub struct UserModel {
    user_id: i32
}

#[wasm_bindgen]
impl UserModel {
    pub fn get_user_id(&self) -> i32 {
        self.user_id
    }
}

#[wasm_bindgen]
pub fn new_user(id:i32) -> UserModel {
    UserModel {user_id: id}
}